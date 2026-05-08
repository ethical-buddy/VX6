package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/vx6/vx6/internal/config"
	"github.com/vx6/vx6/internal/dht"
	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/record"
	"github.com/vx6/vx6/sdk"
)

type state struct {
	mu       sync.Mutex
	client   *sdk.Client
	mode     appMode
	id       identity.Identity
	name     string
	addr     string
	cancel   context.CancelFunc
	contacts map[string]peerContact
	local    localState
	selected int32
	mediaSel int32
}

func main() {
	mode := modeOpen
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) == "org" {
		mode = modeOrg
	}
	st := &state{
		mode:     mode,
		contacts: map[string]peerContact{},
		local: localState{
			Unread:       map[string]int{},
			SeenMessage:  map[string]bool{},
			Pending:      map[string]bool{},
			Delivered:    map[string]bool{},
			ReadByPeer:   map[string]bool{},
			Outbox:       []queuedMessage{},
			ActiveGroups: map[string]groupRoom{},
			SendSeq:      map[string]uint64{},
			RecvSeq:      map[string]uint64{},
		},
		selected: -1,
		mediaSel: -1,
	}

	a := app.NewWithID("com.vx6.comms")
	w := a.NewWindow(windowTitle(mode))
	w.Resize(fyne.NewSize(1240, 820))

	client, err := sdk.New("")
	if err != nil {
		dialog.ShowError(err, w)
		w.ShowAndRun()
		return
	}
	st.client = client
	_ = st.loadIdentityAndConfig()

	topTitle := widget.NewLabelWithStyle("VX6 Comms", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	topSub := widget.NewLabel("Secure decentralized chat with invites, retries, ack tracking, and media transfer")
	statusLabel := widget.NewLabel("Status: idle")
	ipLabel := widget.NewLabel("IPv6: checking")
	refreshIPStatus(ipLabel)

	nameInput := widget.NewEntry()
	nameInput.SetPlaceHolder("Nickname")
	nameInput.SetText(st.name)
	emailInput := widget.NewEntry()
	emailInput.SetPlaceHolder("Email (local profile)")
	phoneInput := widget.NewEntry()
	phoneInput.SetPlaceHolder("Phone (local profile)")

	myInfo := widget.NewMultiLineEntry()
	myInfo.Disable()
	myInfo.SetMinRowsVisible(3)
	refreshMyInfo(st, myInfo)

	startBtn := widget.NewButtonWithIcon("Start Node", theme.MediaPlayIcon(), func() {
		nm := strings.TrimSpace(nameInput.Text)
		if nm == "" {
			dialog.ShowInformation("Name Required", "Please set a nickname.", w)
			return
		}
		if err := st.validateNameUnique(nm); err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := st.initAndStart(nm, emailInput.Text, phoneInput.Text); err != nil {
			dialog.ShowError(err, w)
			return
		}
		statusLabel.SetText("Status: running")
		refreshMyInfo(st, myInfo)
	})
	stopBtn := widget.NewButtonWithIcon("Stop", theme.MediaStopIcon(), func() {
		st.stopNode()
		statusLabel.SetText("Status: stopped")
	})
	renameBtn := widget.NewButton("Rename + Validate", func() {
		nm := strings.TrimSpace(nameInput.Text)
		if nm == "" {
			return
		}
		if err := st.validateNameUnique(nm); err != nil {
			dialog.ShowError(err, w)
			return
		}
		if err := st.renameLocalNode(nm); err != nil {
			dialog.ShowError(err, w)
			return
		}
		refreshMyInfo(st, myInfo)
		dialog.ShowInformation("Name Updated", "Name accepted by network check and updated locally.", w)
	})

	inviteBox := widget.NewMultiLineEntry()
	inviteBox.SetPlaceHolder("Invite link")
	inviteBox.SetMinRowsVisible(4)
	inviteBox.Wrapping = fyne.TextWrapBreak
	genInviteBtn := widget.NewButton("Generate Invite", func() {
		link, err := st.generateInvite()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		inviteBox.SetText(link)
	})

	inviteIn := widget.NewMultiLineEntry()
	inviteIn.SetPlaceHolder("Paste invite link")
	inviteIn.SetMinRowsVisible(4)
	inviteIn.Wrapping = fyne.TextWrapBreak
	addInviteBtn := widget.NewButton("Add Contact", func() {
		if err := st.acceptInvite(inviteIn.Text); err != nil {
			dialog.ShowError(err, w)
			return
		}
		dialog.ShowInformation("Contact Added", "Request sent and contact saved.", w)
	})

	contactsList := widget.NewList(
		func() int { return len(st.sortedContacts()) },
		func() fyne.CanvasObject { return widget.NewLabel("contact") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			cs := st.sortedContacts()
			if i < 0 || i >= len(cs) {
				return
			}
			c := cs[i]
			unread := st.local.Unread[c.NodeID]
			ttl := c.NodeName
			if unread > 0 {
				ttl = fmt.Sprintf("%s (%d)", c.NodeName, unread)
			}
			o.(*widget.Label).SetText(ttl)
		},
	)

	chatLog := widget.NewMultiLineEntry()
	chatLog.Disable()
	chatLog.SetMinRowsVisible(16)
	chatInput := widget.NewMultiLineEntry()
	chatInput.SetMinRowsVisible(4)
	chatInput.SetPlaceHolder("Type message")
	typingLabel := widget.NewLabel("")
	presenceLabel := widget.NewLabel("Presence: unknown")

	chatInput.OnChanged = func(_ string) {
		idx := int(atomic.LoadInt32(&st.selected))
		cs := st.sortedContacts()
		if idx >= 0 && idx < len(cs) {
			_ = st.publishTyping(cs[idx].NodeID, strings.TrimSpace(chatInput.Text) != "")
		}
	}

	sendBtn := widget.NewButton("Send", func() {
		idx := int(atomic.LoadInt32(&st.selected))
		cs := st.sortedContacts()
		if idx < 0 || idx >= len(cs) {
			dialog.ShowInformation("Select Contact", "Pick a contact from left list.", w)
			return
		}
		msg := strings.TrimSpace(chatInput.Text)
		if msg == "" {
			return
		}
		if err := st.sendMessage(cs[idx], msg); err != nil {
			dialog.ShowError(err, w)
			return
		}
		_ = st.publishTyping(cs[idx].NodeID, false)
		chatInput.SetText("")
		_ = st.refreshConversation(cs[idx], chatLog)
	})

	syncBtn := widget.NewButton("Sync", func() {
		if err := st.syncInboxAndRequests(w, chatLog, int(atomic.LoadInt32(&st.selected))); err != nil {
			dialog.ShowError(err, w)
			return
		}
		contactsList.Refresh()
	})

	filePath := widget.NewEntry()
	filePath.SetPlaceHolder("Path to file (video/images/docs)")
	sendFileBtn := widget.NewButton("Send File", func() {
		idx := int(atomic.LoadInt32(&st.selected))
		cs := st.sortedContacts()
		if idx < 0 || idx >= len(cs) {
			dialog.ShowInformation("Select Contact", "Pick a contact first.", w)
			return
		}
		p := strings.TrimSpace(filePath.Text)
		if p == "" {
			return
		}
		progress := dialog.NewProgress("File Transfer", "Sending...", w)
		progress.Show()
		go func() {
			err := st.sendFile(cs[idx], p, func(sent, total int64) {
				fyne.Do(func() {
					if total > 0 {
						progress.SetValue(float64(sent) / float64(total))
					}
				})
			})
			fyne.Do(func() {
				progress.Hide()
				if err != nil {
					dialog.ShowError(err, w)
					return
				}
				dialog.ShowInformation("Transfer Complete", "File sent and metadata announced in chat.", w)
			})
		}()
	})

	callBtn := widget.NewButton("Call Invite", func() {
		idx := int(atomic.LoadInt32(&st.selected))
		cs := st.sortedContacts()
		if idx < 0 || idx >= len(cs) {
			dialog.ShowInformation("Select Contact", "Pick a contact first.", w)
			return
		}
		if err := st.sendCallSignal(cs[idx], "invite"); err != nil {
			dialog.ShowError(err, w)
			return
		}
		dialog.ShowInformation("Call Invite", "Call signal sent. RTP/media plane is next phase.", w)
	})

	mediaList := widget.NewList(
		func() int {
			items, _ := st.listReceivedFiles(40)
			return len(items)
		},
		func() fyne.CanvasObject { return widget.NewLabel("file") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			items, _ := st.listReceivedFiles(40)
			if i < 0 || i >= len(items) {
				return
			}
			o.(*widget.Label).SetText(items[i])
		},
	)
	mediaList.OnSelected = func(id widget.ListItemID) {
		atomic.StoreInt32(&st.mediaSel, int32(id))
	}
	refreshMediaBtn := widget.NewButton("Refresh Inbox", func() { mediaList.Refresh() })
	previewMediaBtn := widget.NewButton("Preview/Open", func() {
		items, _ := st.listReceivedFiles(80)
		idx := int(atomic.LoadInt32(&st.mediaSel))
		if idx < 0 || idx >= len(items) || items[idx] == "(no files yet)" {
			dialog.ShowInformation("Media", "Select a file first.", w)
			return
		}
		st.showMediaPreview(items[idx], w)
	})

	groupName := widget.NewEntry()
	groupName.SetPlaceHolder("Group name")
	groupIDInput := widget.NewEntry()
	groupIDInput.SetPlaceHolder("Group ID")
	groupMemberInput := widget.NewEntry()
	groupMemberInput.SetPlaceHolder("Member NodeID")
	groupMsgInput := widget.NewEntry()
	groupMsgInput.SetPlaceHolder("Group message")
	createGroupBtn := widget.NewButton("Create Group", func() {
		if err := st.createGroup(strings.TrimSpace(groupName.Text)); err != nil {
			dialog.ShowError(err, w)
			return
		}
		groupName.SetText("")
		dialog.ShowInformation("Group Created", "Group metadata published.", w)
	})
	addMemberBtn := widget.NewButton("Add Member", func() {
		if err := st.groupMemberChange(strings.TrimSpace(groupIDInput.Text), strings.TrimSpace(groupMemberInput.Text), "add"); err != nil {
			dialog.ShowError(err, w)
			return
		}
	})
	removeMemberBtn := widget.NewButton("Remove Member", func() {
		if err := st.groupMemberChange(strings.TrimSpace(groupIDInput.Text), strings.TrimSpace(groupMemberInput.Text), "remove"); err != nil {
			dialog.ShowError(err, w)
			return
		}
	})
	sendGroupBtn := widget.NewButton("Send Group Msg", func() {
		if err := st.publishGroupMessage(strings.TrimSpace(groupIDInput.Text), strings.TrimSpace(groupMsgInput.Text)); err != nil {
			dialog.ShowError(err, w)
			return
		}
		groupMsgInput.SetText("")
	})

	contactsList.OnSelected = func(id widget.ListItemID) {
		atomic.StoreInt32(&st.selected, int32(id))
		cs := st.sortedContacts()
		if id >= 0 && id < len(cs) {
			st.local.Unread[cs[id].NodeID] = 0
			_ = st.saveLocalState()
			_ = st.refreshConversation(cs[id], chatLog)
			presenceLabel.SetText("Presence: " + st.peerPresenceSummary(cs[id].NodeID))
		}
	}

	leftPanel := container.NewVBox(
		widget.NewCard("Node", "", container.NewVBox(
			nameInput, emailInput, phoneInput,
			container.NewHBox(startBtn, stopBtn, renameBtn),
			statusLabel, ipLabel,
			widget.NewLabel("My Identity / Address"), myInfo,
		)),
		widget.NewCard("Contacts", "", contactsList),
	)

	centerPanel := container.NewVBox(
		widget.NewCard("Conversation", "", chatLog),
		widget.NewCard("Status", "", container.NewVBox(presenceLabel, typingLabel)),
		container.NewGridWithColumns(2, sendBtn, syncBtn),
		chatInput,
	)

	rightPanel := container.NewVBox(
		widget.NewCard("Invite Link", "", container.NewVBox(genInviteBtn, inviteBox)),
		widget.NewCard("Add Contact", "", container.NewVBox(inviteIn, addInviteBtn)),
		widget.NewCard("Media", "", container.NewVBox(filePath, sendFileBtn, container.NewGridWithColumns(2, refreshMediaBtn, previewMediaBtn), mediaList)),
		widget.NewCard("Groups", "", container.NewVBox(
			groupName, createGroupBtn, groupIDInput, groupMemberInput,
			container.NewGridWithColumns(2, addMemberBtn, removeMemberBtn),
			groupMsgInput, sendGroupBtn,
		)),
		widget.NewCard("Calls", "", container.NewVBox(callBtn)),
	)

	midSplit := container.NewHSplit(centerPanel, rightPanel)
	midSplit.Offset = 0.62
	mainSplit := container.NewHSplit(leftPanel, midSplit)
	mainSplit.Offset = 0.28

	root := container.NewBorder(
		container.NewVBox(topTitle, topSub),
		container.NewHBox(layout.NewSpacer(), widget.NewLabel("VX6 Comms UI")),
		nil, nil,
		mainSplit,
	)
	w.SetContent(root)

	go func() {
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for range t.C {
			_ = st.publishPresence()
			_ = st.syncInboxAndRequests(w, chatLog, int(atomic.LoadInt32(&st.selected)))
			_ = st.retryPending()
			if idx := int(atomic.LoadInt32(&st.selected)); idx >= 0 {
				cs := st.sortedContacts()
				if idx < len(cs) {
					typingLabel.SetText(st.typingSummary(cs[idx].NodeID))
					presenceLabel.SetText("Presence: " + st.peerPresenceSummary(cs[idx].NodeID))
				}
			}
			contactsList.Refresh()
		}
	}()
	w.ShowAndRun()
}

func windowTitle(mode appMode) string {
	if mode == modeOrg {
		return "VX6 Comms (Org)"
	}
	return "VX6 Comms (Open)"
}

func refreshIPStatus(lbl *widget.Label) {
	v6 := false
	ifaces, _ := netInterfaceAddrs()
	for _, a := range ifaces {
		if strings.Contains(a, ":") && !strings.Contains(a, "::1") && !strings.HasPrefix(a, "fe80:") {
			v6 = true
			break
		}
	}
	if v6 {
		lbl.SetText("IPv6: available")
	} else {
		lbl.SetText("IPv6: not detected (relay fallback)")
	}
}

func netInterfaceAddrs() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, 8)
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			out = append(out, addr.String())
		}
	}
	return out, nil
}

func (s *state) initAndStart(name, email, phone string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := s.client.Init(ctx, sdk.InitOptions{Name: name, FileReceiveMode: config.FileReceiveOpen})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
	s.saveProfileMeta(email, phone)
	return s.startNode()
}

func (s *state) renameLocalNode(name string) error {
	store, err := config.NewStore(s.client.ConfigPath())
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err != nil {
		return err
	}
	cfg.Node.Name = name
	if err := store.Save(cfg); err != nil {
		return err
	}
	s.mu.Lock()
	s.name = name
	s.mu.Unlock()
	return nil
}

func (s *state) saveProfileMeta(email, phone string) {
	path := filepath.Join(filepath.Dir(s.client.ConfigPath()), "vx6comms-profile.json")
	_ = os.WriteFile(path, marshalJSON(map[string]string{"email": strings.TrimSpace(email), "phone": strings.TrimSpace(phone)}), 0o644)
}

func (s *state) startNode() error {
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()
	go func() {
		_ = s.client.StartNode(ctx, os.Stdout, sdk.StartOptions{})
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()
	}()
	return nil
}

func (s *state) stopNode() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *state) loadIdentityAndConfig() error {
	store, err := config.NewStore(s.client.ConfigPath())
	if err != nil {
		return err
	}
	cfg, err := store.Load()
	if err == nil {
		s.name = cfg.Node.Name
		s.addr = cfg.Node.AdvertiseAddr
	}
	idStore, err := identity.NewStoreForConfig(store.Path())
	if err != nil {
		return err
	}
	id, err := idStore.Load()
	if err == nil {
		s.id = id
	}
	_ = s.loadContacts()
	_ = s.loadLocalState()
	return nil
}

func (s *state) contactsPath() string {
	return filepath.Join(filepath.Dir(s.client.ConfigPath()), "vx6comms-contacts.json")
}

func (s *state) statePath() string {
	return filepath.Join(filepath.Dir(s.client.ConfigPath()), "vx6comms-state.json")
}

func (s *state) loadContacts() error {
	data, err := os.ReadFile(s.contactsPath())
	if err != nil {
		return nil
	}
	var out map[string]peerContact
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	s.contacts = out
	return nil
}

func (s *state) saveContacts() error {
	return os.WriteFile(s.contactsPath(), marshalJSON(s.contacts), 0o644)
}

func (s *state) loadLocalState() error {
	data, err := os.ReadFile(s.statePath())
	if err != nil {
		return nil
	}
	var st localState
	if err := json.Unmarshal(data, &st); err != nil {
		return err
	}
	if st.Unread == nil {
		st.Unread = map[string]int{}
	}
	if st.SeenMessage == nil {
		st.SeenMessage = map[string]bool{}
	}
	if st.Pending == nil {
		st.Pending = map[string]bool{}
	}
	if st.Delivered == nil {
		st.Delivered = map[string]bool{}
	}
	if st.ReadByPeer == nil {
		st.ReadByPeer = map[string]bool{}
	}
	if st.ActiveGroups == nil {
		st.ActiveGroups = map[string]groupRoom{}
	}
	if st.SendSeq == nil {
		st.SendSeq = map[string]uint64{}
	}
	if st.RecvSeq == nil {
		st.RecvSeq = map[string]uint64{}
	}
	s.local = st
	return nil
}

func (s *state) saveLocalState() error {
	s.local.LastSyncAt = time.Now().UTC().Format(time.RFC3339)
	return os.WriteFile(s.statePath(), marshalJSON(s.local), 0o644)
}

func (s *state) sortedContacts() []peerContact {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]peerContact, 0, len(s.contacts))
	for _, c := range s.contacts {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].NodeName) < strings.ToLower(out[j].NodeName) })
	return out
}

func (s *state) generateInvite() (string, error) {
	if s.id.NodeID == "" || s.name == "" || s.addr == "" {
		if err := s.loadIdentityAndConfig(); err != nil {
			return "", err
		}
	}
	secret, err := randomSecret()
	if err != nil {
		return "", err
	}
	return inviteLink(s.id.NodeID, s.name, s.addr, secret), nil
}

func (s *state) acceptInvite(link string) error {
	req, err := parseInviteLink(link)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.contacts[req.FromID] = peerContact{
		NodeID: req.FromID, NodeName: req.FromName, Address: req.Address, Secret: req.Secret,
		AddedAt: time.Now().UTC().Format(time.RFC3339), Accepted: true, RequestID: req.RequestID,
	}
	s.mu.Unlock()
	_ = s.client.AddPeer(req.FromName, req.Address)
	_ = s.saveContacts()
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	_ = s.client.DHTPut(ctx, requestKey(req.FromID), marshalJSON(friendRequest{
		RequestID: req.RequestID, FromID: s.id.NodeID, FromName: s.name, Address: s.addr, Secret: req.Secret, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}))
	return nil
}

func (s *state) sendMessage(c peerContact, text string) error {
	seq := s.local.SendSeq[c.NodeID] + 1
	env, err := sealMessage(c.Secret, chatMessage{Text: text}, s.id.NodeID, c.NodeID, "msg", seq)
	if err != nil {
		return err
	}
	s.local.SendSeq[c.NodeID] = seq
	if err := s.publishEnvelope(c, env); err != nil {
		s.queueMessage(c.NodeID, env, 1)
		return err
	}
	s.local.Pending[env.ID] = true
	s.queueMessage(c.NodeID, env, 1)
	_ = s.saveLocalState()
	return nil
}

func (s *state) sendFile(c peerContact, p string, onProgress func(sent, total int64)) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s.client.SendFileWithProgress(ctx, c.Address, p, onProgress); err != nil {
		return err
	}
	meta, err := sdk.BuildSharedFile(p, "shared from vx6comms")
	if err != nil {
		return err
	}
	msg := messageEnvelope{
		ID:        "file-" + meta.ID,
		Type:      "media",
		From:      s.id.NodeID,
		To:        c.NodeID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		MediaName: meta.Name,
		MediaSize: meta.Size,
		MediaSHA:  meta.SHA256,
	}
	return s.publishEnvelope(c, msg)
}

func (s *state) publishEnvelope(c peerContact, env messageEnvelope) error {
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	key := pairKey(s.id.NodeID, c.NodeID)
	var ledger conversationLedger
	if raw, err := s.client.DHTGet(ctx, key); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &ledger)
	}
	if s.hasMessageID(ledger.Messages, env.ID) {
		return nil
	}
	ledger.PairKey = key
	ledger.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	ledger.Messages = append(ledger.Messages, env)
	if len(ledger.Messages) > 800 {
		ledger.Messages = ledger.Messages[len(ledger.Messages)-800:]
	}
	return s.client.DHTPut(ctx, key, marshalJSON(ledger))
}

func (s *state) refreshConversation(c peerContact, out *widget.Entry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, pairKey(s.id.NodeID, c.NodeID))
	if err != nil {
		return nil
	}
	var ledger conversationLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return err
	}
	lines := make([]string, 0, len(ledger.Messages))
	lastIncomingID := ""
	for _, m := range ledger.Messages {
		if m.Type == "ack" {
			continue
		}
		if m.Type == "media" {
			src := "Me"
			if m.From != s.id.NodeID {
				src = c.NodeName
			}
			lines = append(lines, fmt.Sprintf("[%s] %s shared file: %s (%d bytes)", m.CreatedAt, src, m.MediaName, m.MediaSize))
			continue
		}
		msg, err := openMessage(c.Secret, m)
		if err != nil {
			continue
		}
		from := "Me"
		if m.From != s.id.NodeID {
			from = c.NodeName
			lastIncomingID = m.ID
		}
		stateMark := ""
		if m.From == s.id.NodeID {
			if s.local.ReadByPeer[m.ID] {
				stateMark = " [read]"
			} else if s.local.Delivered[m.ID] {
				stateMark = " [delivered]"
			} else if s.local.Pending[m.ID] {
				stateMark = " [sending]"
			}
		}
		lines = append(lines, fmt.Sprintf("[%s] %s: %s%s", m.CreatedAt, from, msg.Text, stateMark))
	}
	out.SetText(strings.Join(lines, "\n"))
	if lastIncomingID != "" {
		_ = s.publishReadReceipt(c, lastIncomingID)
	}
	return nil
}

func (s *state) syncInboxAndRequests(win fyne.Window, msgOut *widget.Entry, selected int) error {
	_ = s.checkCallSignals(win)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, requestKey(s.id.NodeID))
	if err == nil && len(raw) > 0 {
		var req friendRequest
		if json.Unmarshal(raw, &req) == nil && req.FromID != "" {
			s.mu.Lock()
			_, exists := s.contacts[req.FromID]
			if !exists {
				s.contacts[req.FromID] = peerContact{
					NodeID: req.FromID, NodeName: req.FromName, Address: req.Address, Secret: req.Secret,
					AddedAt: time.Now().UTC().Format(time.RFC3339), Accepted: true, RequestID: req.RequestID,
				}
				_ = s.saveContacts()
				fyne.Do(func() {
					dialog.ShowInformation("Friend Request", req.FromName+" sent a request and was added.", win)
				})
			}
			s.mu.Unlock()
		}
	}

	cs := s.sortedContacts()
	for _, c := range cs {
		_ = s.syncContactLedger(c)
	}
	if selected >= 0 && selected < len(cs) {
		_ = s.refreshConversation(cs[selected], msgOut)
	}
	_ = s.saveLocalState()
	return nil
}

func (s *state) syncContactLedger(c peerContact) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, pairKey(s.id.NodeID, c.NodeID))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var ledger conversationLedger
	if err := json.Unmarshal(raw, &ledger); err != nil {
		return err
	}
	hasNew := false
	for _, m := range ledger.Messages {
		if m.Type == "ack" {
			delete(s.local.Pending, m.AckFor)
			s.local.Delivered[m.AckFor] = true
			continue
		}
		if m.Type == "read" {
			s.local.ReadByPeer[m.AckFor] = true
			continue
		}
		if s.local.SeenMessage[m.ID] {
			continue
		}
		if m.From == c.NodeID && m.Seq > 0 {
			if m.Seq <= s.local.RecvSeq[c.NodeID] {
				continue
			}
			s.local.RecvSeq[c.NodeID] = m.Seq
		}
		s.local.SeenMessage[m.ID] = true
		if m.From != s.id.NodeID {
			s.local.Unread[c.NodeID] = s.local.Unread[c.NodeID] + 1
			ack := makeAckMessage(m.ID, s.id.NodeID, c.NodeID)
			_ = s.publishEnvelope(c, ack)
		}
		hasNew = true
	}
	if hasNew {
		_ = s.saveLocalState()
	}
	return nil
}

func (s *state) retryPending() error {
	now := time.Now().UTC()
	nextOut := make([]queuedMessage, 0, len(s.local.Outbox))
	for _, q := range s.local.Outbox {
		if !s.local.Pending[q.Envelope.ID] {
			continue
		}
		when, _ := time.Parse(time.RFC3339, q.NextRetry)
		if when.After(now) {
			nextOut = append(nextOut, q)
			continue
		}
		c, ok := s.contacts[q.ContactID]
		if !ok {
			continue
		}
		_ = s.publishEnvelope(c, q.Envelope)
		q.Retries++
		if q.Retries <= 5 {
			q.NextRetry = now.Add(time.Duration(4+q.Retries*2) * time.Second).Format(time.RFC3339)
			nextOut = append(nextOut, q)
		}
	}
	s.local.Outbox = nextOut
	return s.saveLocalState()
}

func (s *state) queueMessage(contactID string, env messageEnvelope, delaySeconds int) {
	for _, q := range s.local.Outbox {
		if q.Envelope.ID == env.ID {
			return
		}
	}
	s.local.Outbox = append(s.local.Outbox, queuedMessage{
		ContactID: contactID,
		Envelope:  env,
		Retries:   0,
		NextRetry: time.Now().UTC().Add(time.Duration(delaySeconds) * time.Second).Format(time.RFC3339),
	})
}

func (s *state) hasMessageID(items []messageEnvelope, id string) bool {
	for _, m := range items {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (s *state) createGroup(name string) error {
	if name == "" {
		return fmt.Errorf("group name required")
	}
	secret, err := randomSecret()
	if err != nil {
		return err
	}
	id := fmt.Sprintf("grp-%d", time.Now().UnixNano())
	gr := groupRoom{
		ID: id, Name: name, Secret: secret, Members: []string{s.id.NodeID}, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.local.ActiveGroups[id] = gr
	_ = s.saveLocalState()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	ledger := groupLedger{
		GroupID: id, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Events: []groupEvent{{
			ID: fmt.Sprintf("gev-%d", time.Now().UnixNano()), GroupID: id, Type: "create",
			ActorID: s.id.NodeID, Payload: name, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}},
	}
	return s.client.DHTPut(ctx, groupKey(id), marshalJSON(ledger))
}

func (s *state) groupMemberChange(groupID, targetID, action string) error {
	if groupID == "" || targetID == "" {
		return fmt.Errorf("group id and target node id required")
	}
	if action != "add" && action != "remove" && action != "promote" && action != "demote" {
		return fmt.Errorf("invalid group action")
	}
	ledger, err := s.loadGroupLedger(groupID)
	if err != nil {
		return err
	}
	members, admins := groupStateFromLedger(ledger)
	if !admins[s.id.NodeID] {
		return fmt.Errorf("only admins can change membership")
	}
	if action == "add" && members[targetID] {
		return fmt.Errorf("member already exists")
	}
	if action == "remove" && !members[targetID] {
		return fmt.Errorf("member not found")
	}
	ledger.Events = append(ledger.Events, groupEvent{
		ID: fmt.Sprintf("gev-%d", time.Now().UnixNano()), GroupID: groupID, Type: action,
		ActorID: s.id.NodeID, TargetID: targetID, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	ledger.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveGroupLedger(groupID, ledger)
}

func (s *state) publishGroupMessage(groupID, text string) error {
	if groupID == "" || text == "" {
		return fmt.Errorf("group id and text required")
	}
	ledger, err := s.loadGroupLedger(groupID)
	if err != nil {
		return err
	}
	members, _ := groupStateFromLedger(ledger)
	if !members[s.id.NodeID] {
		return fmt.Errorf("only group members can send group messages")
	}
	ledger.Events = append(ledger.Events, groupEvent{
		ID: fmt.Sprintf("gev-%d", time.Now().UnixNano()), GroupID: groupID, Type: "msg",
		ActorID: s.id.NodeID, Payload: text, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	ledger.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.saveGroupLedger(groupID, ledger)
}

func (s *state) loadGroupLedger(groupID string) (groupLedger, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	var ledger groupLedger
	raw, err := s.client.DHTGet(ctx, groupKey(groupID))
	if err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &ledger)
	}
	if ledger.GroupID == "" {
		ledger.GroupID = groupID
		ledger.Events = []groupEvent{}
	}
	return ledger, nil
}

func (s *state) saveGroupLedger(groupID string, ledger groupLedger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	return s.client.DHTPut(ctx, groupKey(groupID), marshalJSON(ledger))
}

func groupStateFromLedger(ledger groupLedger) (map[string]bool, map[string]bool) {
	members := map[string]bool{}
	admins := map[string]bool{}
	for _, ev := range ledger.Events {
		switch ev.Type {
		case "create":
			members[ev.ActorID] = true
			admins[ev.ActorID] = true
		case "add":
			if ev.TargetID != "" {
				members[ev.TargetID] = true
			}
		case "remove":
			delete(members, ev.TargetID)
			delete(admins, ev.TargetID)
		case "promote":
			if members[ev.TargetID] {
				admins[ev.TargetID] = true
			}
		case "demote":
			delete(admins, ev.TargetID)
		}
	}
	return members, admins
}

func (s *state) publishReadReceipt(c peerContact, msgID string) error {
	ack := makeAckMessage(msgID, s.id.NodeID, c.NodeID)
	ack.Type = "read"
	return s.publishEnvelope(c, ack)
}

func (s *state) publishPresence() error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	ps := presenceState{
		NodeID: s.id.NodeID, NodeName: s.name, DeviceID: s.id.NodeID, Status: "online",
		LastSeen: time.Now().UTC().Format(time.RFC3339), Transport: "vx6",
	}
	return s.client.DHTPut(ctx, presenceKey(s.id.NodeID), marshalJSON(ps))
}

func (s *state) peerPresenceSummary(nodeID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, presenceKey(nodeID))
	if err != nil || len(raw) == 0 {
		return "offline/unknown"
	}
	var ps presenceState
	if err := json.Unmarshal(raw, &ps); err != nil {
		return "offline/unknown"
	}
	return ps.Status + " @ " + ps.LastSeen
}

func (s *state) publishTyping(toNodeID string, typing bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ts := typingState{
		From: s.id.NodeID, To: toNodeID, IsTyping: typing, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return s.client.DHTPut(ctx, typingKey(s.id.NodeID, toNodeID), marshalJSON(ts))
}

func (s *state) typingSummary(peerNodeID string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, typingKey(s.id.NodeID, peerNodeID))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var ts typingState
	if err := json.Unmarshal(raw, &ts); err != nil {
		return ""
	}
	if ts.From == peerNodeID && ts.IsTyping {
		return "Typing..."
	}
	return ""
}

func (s *state) sendCallSignal(c peerContact, action string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	sig := callSignal{
		ID: fmt.Sprintf("call-%d", time.Now().UnixNano()), FromID: s.id.NodeID, FromName: s.name,
		ToID: c.NodeID, Action: action, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return s.client.DHTPut(ctx, callSignalKey(c.NodeID), marshalJSON(sig))
}

func (s *state) checkCallSignals(win fyne.Window) error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, callSignalKey(s.id.NodeID))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var sig callSignal
	if err := json.Unmarshal(raw, &sig); err != nil {
		return nil
	}
	if sig.ToID != s.id.NodeID || sig.Action != "invite" {
		return nil
	}
	fyne.Do(func() {
		dialog.ShowConfirm("Incoming Call", sig.FromName+" is inviting you to a VX6 call. Accept signaling?", func(ok bool) {
			peer := s.findContactByID(sig.FromID)
			if peer.NodeID == "" {
				return
			}
			if ok {
				_ = s.sendCallSignal(peer, "accept")
				dialog.ShowInformation("Call", "Accepted. RTP/WebRTC media channel wiring is next.", win)
			} else {
				_ = s.sendCallSignal(peer, "decline")
			}
		}, win)
	})
	return nil
}

func (s *state) findContactByID(id string) peerContact {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contacts[id]
}

func (s *state) listReceivedFiles(limit int) ([]string, error) {
	store, err := config.NewStore(s.client.ConfigPath())
	if err != nil {
		return nil, err
	}
	cfg, err := store.Load()
	if err != nil {
		return nil, err
	}
	root := cfg.Node.DownloadDir
	if root == "" {
		root, _ = config.DefaultDownloadDir()
	}
	entries := make([]string, 0, limit)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		entries = append(entries, path)
		if len(entries) >= limit {
			return fmt.Errorf("stop")
		}
		return nil
	})
	if len(entries) == 0 {
		return []string{"(no files yet)"}, nil
	}
	return entries, nil
}

func (s *state) showMediaPreview(path string, win fyne.Window) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		img := canvas.NewImageFromFile(path)
		img.FillMode = canvas.ImageFillContain
		img.SetMinSize(fyne.NewSize(640, 420))
		dialog.ShowCustom("Image Preview", "Close", container.NewStack(
			canvas.NewRectangle(color.Black),
			img,
		), win)
	case ".mp4", ".mkv", ".webm", ".mov":
		dialog.ShowInformation("Video Preview", "Video inline playback is not wired yet. File path:\n"+path, win)
	default:
		dialog.ShowInformation("File", path, win)
	}
}

func refreshMyInfo(s *state, out *widget.Entry) {
	_ = s.loadIdentityAndConfig()
	out.SetText(fmt.Sprintf("Node Name: %s\nNode ID: %s\nAddress: %s", s.name, s.id.NodeID, s.addr))
}

func (s *state) validateNameUnique(name string) error {
	if err := record.ValidateNodeName(name); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	raw, err := s.client.DHTGet(ctx, dht.NodeNameKey(name))
	if err != nil || len(raw) == 0 {
		return nil
	}
	var rec record.EndpointRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil
	}
	if rec.NodeID != "" && s.id.NodeID != "" && rec.NodeID != s.id.NodeID {
		return fmt.Errorf("name already exists in network with different node key (%s). choose another", rec.NodeID)
	}
	return nil
}
