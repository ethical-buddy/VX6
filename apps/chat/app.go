package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Options struct {
	ConfigPath      string
	HTTPAddr        string
	TransportAddr   string
	PublishInterval time.Duration
}

type App struct {
	log io.Writer

	rt    runtimeContext
	store *chatStore
	opts  Options

	httpServer *http.Server
	httpAddr   string

	transportAddr string

	eventsMu sync.Mutex
	events   map[chan []byte]struct{}
}

func Run(ctx context.Context, log io.Writer, opts Options) error {
	app, err := New(log, opts)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func New(log io.Writer, opts Options) (*App, error) {
	rt, err := loadRuntime(opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	store, err := newChatStore(rt.statePath)
	if err != nil {
		return nil, err
	}
	return &App{
		log:    log,
		rt:     rt,
		store:  store,
		opts:   opts,
		events: map[chan []byte]struct{}{},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if a.opts.PublishInterval <= 0 {
		a.opts.PublishInterval = 5 * time.Minute
	}
	if err := a.startTransport(ctx); err != nil {
		return err
	}
	changed, err := ensureChatService(a.rt.store, a.transportAddr)
	if err != nil {
		return err
	}
	if changed {
		_ = signalVX6Reload(a.rt.store)
	}
	if err := publishChatService(ctx, a.rt); err != nil {
		return err
	}
	if err := a.startHTTP(ctx); err != nil {
		return err
	}
	a.broadcastSnapshot()

	go a.republishLoop(ctx)
	go a.retryLoop(ctx)

	fmt.Fprintf(a.log, "vx6-chat\tweb=%s\ttransport=%s\tuser=%s\n", a.httpAddr, a.transportAddr, a.rt.cfg.Node.Name)
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return a.httpServer.Shutdown(shutdownCtx)
}

func (a *App) startHTTP(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/events", a.handleEvents)
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/conversations/dm", a.handleCreateDM)
	mux.HandleFunc("/api/conversations/group", a.handleCreateGroup)
	mux.HandleFunc("/api/messages", a.handleSendMessage)

	ln, err := net.Listen("tcp", a.opts.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen web UI: %w", err)
	}
	a.httpAddr = ln.Addr().String()
	a.httpServer = &http.Server{Handler: mux}

	go func() {
		<-ctx.Done()
		_ = a.httpServer.Close()
	}()
	go func() {
		_ = a.httpServer.Serve(ln)
	}()
	return nil
}

func (a *App) startTransport(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.opts.TransportAddr)
	if err != nil {
		return fmt.Errorf("listen chat transport: %w", err)
	}
	a.transportAddr = ln.Addr().String()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go a.handleTransportConn(conn)
		}
	}()
	return nil
}

func (a *App) republishLoop(ctx context.Context) {
	ticker := time.NewTicker(a.opts.PublishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = publishChatService(ctx, a.rt)
		}
	}
}

func (a *App) retryLoop(ctx context.Context) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.flushPending(ctx)
		}
	}
}

func (a *App) flushPending(ctx context.Context) {
	pending := a.store.dueDeliveries(time.Now())
	if len(pending) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, item := range pending {
		item := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
			defer cancel()
			err := a.deliver(sendCtx, item.Peer, item.Message)
			if err == nil {
				_ = a.store.markDelivered(item.Message.ID, item.Peer)
				a.broadcastSnapshot()
				return
			}
			attempts := item.Attempts + 1
			delay := time.Duration(attempts*attempts) * 2 * time.Second
			if delay > time.Minute {
				delay = time.Minute
			}
			_ = a.store.markDeliveryFailure(item.Message.ID, item.Peer, attempts, time.Now().Add(delay), err)
			a.broadcastSnapshot()
		}()
	}
	wg.Wait()
}

func (a *App) deliver(ctx context.Context, peer string, msg *Message) error {
	conn, err := dialChatPeer(ctx, a.rt, peer)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ackTimeout))

	if err := writeEnvelope(conn, wireEnvelope{
		Type:    "message",
		Message: cloneMessage(msg),
	}); err != nil {
		return err
	}

	reply, err := readEnvelope(conn)
	if err != nil {
		return err
	}
	if reply.Type == "ack" && reply.AckID == msg.ID {
		return nil
	}
	if reply.Error != "" {
		return errors.New(reply.Error)
	}
	return errors.New("unexpected chat ack")
}

func (a *App) handleTransportConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ackTimeout))

	for {
		env, err := readEnvelope(conn)
		if err != nil {
			return
		}
		if env.Type != "message" || env.Message == nil {
			_ = writeEnvelope(conn, wireEnvelope{Type: "error", Error: "unsupported chat frame"})
			return
		}
		if err := validateIncomingMessage(env.Message); err != nil {
			_ = writeEnvelope(conn, wireEnvelope{Type: "error", Error: err.Error()})
			return
		}
		_, err = a.store.acceptRemoteMessage(env.Message)
		if err != nil {
			_ = writeEnvelope(conn, wireEnvelope{Type: "error", Error: err.Error()})
			return
		}
		a.broadcastSnapshot()
		if err := writeEnvelope(conn, wireEnvelope{Type: "ack", AckID: env.Message.ID}); err != nil {
			return
		}
	}
}

func validateIncomingMessage(msg *Message) error {
	if msg == nil {
		return errors.New("missing message")
	}
	if msg.ID == "" || msg.ConversationID == "" || msg.From == "" {
		return errors.New("message missing required fields")
	}
	if strings.TrimSpace(msg.Body) == "" {
		return errors.New("message body is empty")
	}
	if len(msg.Body) > 4000 {
		return errors.New("message body is too large")
	}
	switch msg.Kind {
	case "dm", "group":
	default:
		return errors.New("invalid conversation kind")
	}
	return nil
}

func (a *App) newDirectMessage(peer, body string) (*Message, []string, error) {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return nil, nil, errors.New("peer is required")
	}
	conv, err := a.store.ensureDM(a.rt.cfg.Node.Name, peer)
	if err != nil {
		return nil, nil, err
	}
	msg := &Message{
		ID:             randomID(),
		ConversationID: conv.ID,
		Kind:           "dm",
		Title:          conv.Title,
		Members:        conv.Members,
		From:           a.rt.cfg.Node.Name,
		Body:           strings.TrimSpace(body),
		SentAt:         time.Now().UTC().Format(time.RFC3339),
	}
	return msg, []string{peer}, nil
}

func (a *App) createGroup(title string, members []string) (*Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("group title is required")
	}
	members = append([]string{a.rt.cfg.Node.Name}, members...)
	return a.store.createGroup(title, members)
}

func (a *App) sendConversationMessage(ctx context.Context, conversationID, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return errors.New("message body is empty")
	}

	view := a.currentState()
	var conv *Conversation
	for _, candidate := range view.Conversations {
		if candidate.ID == conversationID {
			conv = candidate
			break
		}
	}
	if conv == nil {
		return errors.New("conversation not found")
	}

	recipients := make([]string, 0, len(conv.Members))
	for _, member := range conv.Members {
		if member == a.rt.cfg.Node.Name {
			continue
		}
		recipients = append(recipients, member)
	}
	msg := &Message{
		ID:             randomID(),
		ConversationID: conv.ID,
		Kind:           conv.Kind,
		Title:          conv.Title,
		Members:        conv.Members,
		From:           a.rt.cfg.Node.Name,
		Body:           body,
		SentAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if err := a.store.addLocalMessage(msg, recipients); err != nil {
		return err
	}
	a.broadcastSnapshot()

	var wg sync.WaitGroup
	for _, peer := range recipients {
		peer := peer
		wg.Add(1)
		go func() {
			defer wg.Done()
			sendCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
			defer cancel()
			err := a.deliver(sendCtx, peer, msg)
			if err == nil {
				_ = a.store.markDelivered(msg.ID, peer)
			} else {
				_ = a.store.markDeliveryFailure(msg.ID, peer, 1, time.Now().Add(6*time.Second), err)
			}
			a.broadcastSnapshot()
		}()
	}
	wg.Wait()
	return nil
}

func (a *App) currentState() StateView {
	return a.store.snapshot(a.rt.cfg.Node.Name, a.httpAddr, a.transportAddr, peerViews(a.rt.cfg.Node.Name, a.rt.registry))
}

func (a *App) broadcastSnapshot() {
	snapshot := a.currentState()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return
	}

	a.eventsMu.Lock()
	defer a.eventsMu.Unlock()
	for ch := range a.events {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (a *App) subscribe() chan []byte {
	ch := make(chan []byte, 4)
	a.eventsMu.Lock()
	a.events[ch] = struct{}{}
	a.eventsMu.Unlock()
	return ch
}

func (a *App) unsubscribe(ch chan []byte) {
	a.eventsMu.Lock()
	delete(a.events, ch)
	a.eventsMu.Unlock()
	close(ch)
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (a *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := a.subscribe()
	defer a.unsubscribe(ch)

	initial, _ := json.Marshal(a.currentState())
	_, _ = fmt.Fprintf(w, "data: %s\n\n", initial)
	flusher.Flush()

	keepAlive := time.NewTicker(20 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		}
	}
}

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.currentState())
}

func (a *App) handleCreateDM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Peer string `json:"peer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conv, err := a.store.ensureDM(a.rt.cfg.Node.Name, req.Peer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.broadcastSnapshot()
	writeJSON(w, http.StatusOK, conv)
}

func (a *App) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Title   string   `json:"title"`
		Members []string `json:"members"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	conv, err := a.createGroup(req.Title, req.Members)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.broadcastSnapshot()
	writeJSON(w, http.StatusOK, conv)
}

func (a *App) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ConversationID string `json:"conversation_id"`
		Body           string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.sendConversationMessage(r.Context(), req.ConversationID, req.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
