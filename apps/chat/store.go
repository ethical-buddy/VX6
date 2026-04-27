package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type chatStore struct {
	path string

	mu    sync.RWMutex
	state persistedState
}

type persistedState struct {
	Conversations map[string]*Conversation `json:"conversations"`
	Pending       []*PendingDelivery       `json:"pending"`
}

type Conversation struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Members   []string   `json:"members"`
	UpdatedAt string     `json:"updated_at"`
	Messages  []*Message `json:"messages"`
}

type Message struct {
	ID             string   `json:"id"`
	ConversationID string   `json:"conversation_id"`
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Members        []string `json:"members"`
	From           string   `json:"from"`
	Body           string   `json:"body"`
	SentAt         string   `json:"sent_at"`
}

type PendingDelivery struct {
	Message     *Message `json:"message"`
	Peer        string   `json:"peer"`
	Attempts    int      `json:"attempts"`
	NextAttempt string   `json:"next_attempt"`
	LastError   string   `json:"last_error,omitempty"`
}

type PeerView struct {
	Name    string `json:"name"`
	HasChat bool   `json:"has_chat"`
}

type StateView struct {
	Self          string          `json:"self"`
	WebAddr       string          `json:"web_addr"`
	TransportAddr string          `json:"transport_addr"`
	Conversations []*Conversation `json:"conversations"`
	Peers         []PeerView      `json:"peers"`
}

func newChatStore(path string) (*chatStore, error) {
	s := &chatStore{
		path: path,
		state: persistedState{
			Conversations: map[string]*Conversation{},
			Pending:       []*PendingDelivery{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *chatStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read chat state: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("decode chat state: %w", err)
	}
	if state.Conversations == nil {
		state.Conversations = map[string]*Conversation{}
	}
	if state.Pending == nil {
		state.Pending = []*PendingDelivery{}
	}
	for _, conv := range state.Conversations {
		normalizeConversation(conv)
	}
	s.state = state
	return nil
}

func (s *chatStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create chat state directory: %w", err)
	}
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode chat state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write chat state: %w", err)
	}
	return nil
}

func (s *chatStore) ensureDM(self, peer string) (*Conversation, error) {
	conv := &Conversation{
		ID:      dmConversationID(self, peer),
		Kind:    "dm",
		Title:   peer,
		Members: sortedUniqueMembers([]string{self, peer}),
	}
	return s.ensureConversation(conv)
}

func (s *chatStore) createGroup(title string, members []string) (*Conversation, error) {
	conv := &Conversation{
		ID:      "group_" + randomID(),
		Kind:    "group",
		Title:   strings.TrimSpace(title),
		Members: sortedUniqueMembers(members),
	}
	return s.ensureConversation(conv)
}

func (s *chatStore) ensureConversation(conv *Conversation) (*Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalizeConversation(conv)
	existing, ok := s.state.Conversations[conv.ID]
	if !ok {
		cp := cloneConversation(conv)
		s.state.Conversations[conv.ID] = cp
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
		return cloneConversation(cp), nil
	}

	changed := false
	if existing.Kind == "" {
		existing.Kind = conv.Kind
		changed = true
	}
	if conv.Title != "" && existing.Title != conv.Title {
		existing.Title = conv.Title
		changed = true
	}
	mergedMembers := sortedUniqueMembers(append(existing.Members, conv.Members...))
	if !equalStrings(existing.Members, mergedMembers) {
		existing.Members = mergedMembers
		changed = true
	}
	if changed {
		normalizeConversation(existing)
		if err := s.saveLocked(); err != nil {
			return nil, err
		}
	}
	return cloneConversation(existing), nil
}

func (s *chatStore) addLocalMessage(msg *Message, recipients []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.addMessageLocked(msg); err != nil {
		return err
	}
	for _, peer := range sortedUniqueMembers(recipients) {
		if peer == msg.From {
			continue
		}
		s.state.Pending = append(s.state.Pending, &PendingDelivery{
			Message:     cloneMessage(msg),
			Peer:        peer,
			Attempts:    0,
			NextAttempt: time.Now().UTC().Format(time.RFC3339),
		})
	}
	return s.saveLocked()
}

func (s *chatStore) acceptRemoteMessage(msg *Message) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	added, err := s.addMessageLocked(msg)
	if err != nil {
		return false, err
	}
	if !added {
		return false, nil
	}
	return true, s.saveLocked()
}

func (s *chatStore) addMessageLocked(msg *Message) (bool, error) {
	normalizeMessage(msg)
	conv, ok := s.state.Conversations[msg.ConversationID]
	if !ok {
		conv = &Conversation{
			ID:       msg.ConversationID,
			Kind:     msg.Kind,
			Title:    msg.Title,
			Members:  sortedUniqueMembers(msg.Members),
			Messages: []*Message{},
		}
		normalizeConversation(conv)
		s.state.Conversations[msg.ConversationID] = conv
	}
	if conv.Kind == "" {
		conv.Kind = msg.Kind
	}
	if msg.Title != "" {
		conv.Title = msg.Title
	}
	conv.Members = sortedUniqueMembers(append(conv.Members, msg.Members...))
	for _, existing := range conv.Messages {
		if existing.ID == msg.ID {
			return false, nil
		}
	}
	conv.Messages = append(conv.Messages, cloneMessage(msg))
	sort.SliceStable(conv.Messages, func(i, j int) bool {
		return conv.Messages[i].SentAt < conv.Messages[j].SentAt
	})
	conv.UpdatedAt = msg.SentAt
	return true, nil
}

func (s *chatStore) dueDeliveries(now time.Time) []*PendingDelivery {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var due []*PendingDelivery
	for _, pending := range s.state.Pending {
		next, err := time.Parse(time.RFC3339, pending.NextAttempt)
		if err != nil || !next.After(now) {
			due = append(due, clonePending(pending))
		}
	}
	return due
}

func (s *chatStore) markDelivered(messageID, peer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.state.Pending[:0]
	for _, pending := range s.state.Pending {
		if pending.Message != nil && pending.Message.ID == messageID && pending.Peer == peer {
			continue
		}
		filtered = append(filtered, pending)
	}
	s.state.Pending = filtered
	return s.saveLocked()
}

func (s *chatStore) markDeliveryFailure(messageID, peer string, attempts int, next time.Time, err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pending := range s.state.Pending {
		if pending.Message == nil {
			continue
		}
		if pending.Message.ID == messageID && pending.Peer == peer {
			pending.Attempts = attempts
			pending.NextAttempt = next.UTC().Format(time.RFC3339)
			if err != nil {
				pending.LastError = err.Error()
			}
			return s.saveLocked()
		}
	}
	return nil
}

func (s *chatStore) snapshot(self, webAddr, transportAddr string, peers []PeerView) StateView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conversations := make([]*Conversation, 0, len(s.state.Conversations))
	for _, conv := range s.state.Conversations {
		conversations = append(conversations, cloneConversation(conv))
	}
	sort.SliceStable(conversations, func(i, j int) bool {
		return conversations[i].UpdatedAt > conversations[j].UpdatedAt
	})

	sort.SliceStable(peers, func(i, j int) bool {
		if peers[i].HasChat != peers[j].HasChat {
			return peers[i].HasChat
		}
		return peers[i].Name < peers[j].Name
	})

	return StateView{
		Self:          self,
		WebAddr:       webAddr,
		TransportAddr: transportAddr,
		Conversations: conversations,
		Peers:         peers,
	}
}

func normalizeConversation(conv *Conversation) {
	if conv == nil {
		return
	}
	if conv.Messages == nil {
		conv.Messages = []*Message{}
	}
	conv.Members = sortedUniqueMembers(conv.Members)
	if conv.UpdatedAt == "" {
		if len(conv.Messages) > 0 {
			conv.UpdatedAt = conv.Messages[len(conv.Messages)-1].SentAt
		} else {
			conv.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
}

func normalizeMessage(msg *Message) {
	msg.Members = sortedUniqueMembers(msg.Members)
	if msg.SentAt == "" {
		msg.SentAt = time.Now().UTC().Format(time.RFC3339)
	}
}

func cloneConversation(in *Conversation) *Conversation {
	if in == nil {
		return nil
	}
	out := *in
	out.Members = append([]string(nil), in.Members...)
	out.Messages = make([]*Message, 0, len(in.Messages))
	for _, msg := range in.Messages {
		out.Messages = append(out.Messages, cloneMessage(msg))
	}
	return &out
}

func cloneMessage(in *Message) *Message {
	if in == nil {
		return nil
	}
	out := *in
	out.Members = append([]string(nil), in.Members...)
	return &out
}

func clonePending(in *PendingDelivery) *PendingDelivery {
	if in == nil {
		return nil
	}
	out := *in
	out.Message = cloneMessage(in.Message)
	return &out
}

func dmConversationID(a, b string) string {
	pair := sortedUniqueMembers([]string{a, b})
	return "dm:" + strings.Join(pair, ":")
}

func sortedUniqueMembers(members []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(members))
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member == "" {
			continue
		}
		if _, ok := seen[member]; ok {
			continue
		}
		seen[member] = struct{}{}
		out = append(out, member)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func randomID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
