package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type appMode string

const (
	modeOpen appMode = "open"
	modeOrg  appMode = "org"
)

type peerContact struct {
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	Address   string `json:"address"`
	Secret    string `json:"secret"`
	AddedAt   string `json:"added_at"`
	Accepted  bool   `json:"accepted"`
	RequestID string `json:"request_id"`
}

type messageEnvelope struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Seq       uint64 `json:"seq,omitempty"`
	AckFor    string `json:"ack_for,omitempty"`
	MediaName string `json:"media_name,omitempty"`
	MediaSize int64  `json:"media_size,omitempty"`
	MediaSHA  string `json:"media_sha,omitempty"`
	GroupID   string `json:"group_id,omitempty"`
	From      string `json:"from"`
	To        string `json:"to"`
	CreatedAt string `json:"created_at"`
	Nonce     string `json:"nonce"`
	Cipher    string `json:"cipher"`
}

type conversationLedger struct {
	PairKey   string            `json:"pair_key"`
	UpdatedAt string            `json:"updated_at"`
	Messages  []messageEnvelope `json:"messages"`
}

type chatMessage struct {
	Text string `json:"text"`
}

type friendRequest struct {
	RequestID string `json:"request_id"`
	FromID    string `json:"from_id"`
	FromName  string `json:"from_name"`
	Address   string `json:"address"`
	Secret    string `json:"secret"`
	CreatedAt string `json:"created_at"`
}

type groupRoom struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Secret    string   `json:"secret"`
	Members   []string `json:"members"`
	CreatedAt string   `json:"created_at"`
}

type queuedMessage struct {
	ContactID string          `json:"contact_id"`
	Envelope  messageEnvelope `json:"envelope"`
	Retries   int             `json:"retries"`
	NextRetry string          `json:"next_retry"`
}

type localState struct {
	Unread       map[string]int       `json:"unread"`
	SeenMessage  map[string]bool      `json:"seen_message"`
	Pending      map[string]bool      `json:"pending"`
	Delivered    map[string]bool      `json:"delivered"`
	ReadByPeer   map[string]bool      `json:"read_by_peer"`
	Outbox       []queuedMessage      `json:"outbox"`
	LastSyncAt   string               `json:"last_sync_at"`
	ActiveGroups map[string]groupRoom `json:"active_groups"`
	SendSeq      map[string]uint64    `json:"send_seq"`
	RecvSeq      map[string]uint64    `json:"recv_seq"`
}

type presenceState struct {
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	DeviceID  string `json:"device_id"`
	Status    string `json:"status"`
	LastSeen  string `json:"last_seen"`
	Transport string `json:"transport"`
}

type typingState struct {
	From      string `json:"from"`
	To        string `json:"to"`
	IsTyping  bool   `json:"is_typing"`
	UpdatedAt string `json:"updated_at"`
}

type groupEvent struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	Type      string `json:"type"` // create|add|remove|promote|demote|msg
	ActorID   string `json:"actor_id"`
	TargetID  string `json:"target_id,omitempty"`
	Payload   string `json:"payload,omitempty"`
	CreatedAt string `json:"created_at"`
}

type groupLedger struct {
	GroupID   string       `json:"group_id"`
	UpdatedAt string       `json:"updated_at"`
	Events    []groupEvent `json:"events"`
}

type callSignal struct {
	ID        string `json:"id"`
	FromID    string `json:"from_id"`
	FromName  string `json:"from_name"`
	ToID      string `json:"to_id"`
	Action    string `json:"action"` // invite|accept|decline|hangup
	CreatedAt string `json:"created_at"`
}

func pairKey(a, b string) string {
	ids := []string{a, b}
	sort.Strings(ids)
	return "vx6chat/conv/" + ids[0] + "/" + ids[1]
}

func requestKey(toNodeID string) string {
	return "vx6chat/request/" + toNodeID
}

func groupKey(groupID string) string {
	return "vx6chat/group/" + groupID
}

func presenceKey(nodeID string) string {
	return "vx6chat/presence/" + nodeID
}

func typingKey(a, b string) string {
	ids := []string{a, b}
	sort.Strings(ids)
	return "vx6chat/typing/" + ids[0] + "/" + ids[1]
}

func callSignalKey(nodeID string) string {
	return "vx6chat/call/" + nodeID
}

func marshalJSON(v any) []byte {
	out, _ := json.Marshal(v)
	return out
}

func sealMessage(secret string, plain chatMessage, from, to, kind string, seq uint64) (messageEnvelope, error) {
	raw, err := json.Marshal(plain)
	if err != nil {
		return messageEnvelope{}, err
	}
	key := deriveMessageKey(secret, from, to, seq)
	block, err := aes.NewCipher(key)
	if err != nil {
		return messageEnvelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return messageEnvelope{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return messageEnvelope{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte(from+"\n"+to))
	sum := sha256.Sum256(append(nonce, ciphertext...))
	return messageEnvelope{
		ID:        base64.RawURLEncoding.EncodeToString(sum[:12]),
		Type:      kind,
		Seq:       seq,
		From:      from,
		To:        to,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Nonce:     base64.RawURLEncoding.EncodeToString(nonce),
		Cipher:    base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func makeAckMessage(ackedID, from, to string) messageEnvelope {
	return messageEnvelope{
		ID:        base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("ack-%s-%d", ackedID, time.Now().UnixNano()))),
		Type:      "ack",
		AckFor:    ackedID,
		From:      from,
		To:        to,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func openMessage(secret string, env messageEnvelope) (chatMessage, error) {
	key := deriveMessageKey(secret, env.From, env.To, env.Seq)
	block, err := aes.NewCipher(key)
	if err != nil {
		return chatMessage{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return chatMessage{}, err
	}
	nonce, err := base64.RawURLEncoding.DecodeString(env.Nonce)
	if err != nil {
		return chatMessage{}, err
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(env.Cipher)
	if err != nil {
		return chatMessage{}, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(env.From+"\n"+env.To))
	if err != nil {
		return chatMessage{}, err
	}
	var msg chatMessage
	if err := json.Unmarshal(plain, &msg); err != nil {
		return chatMessage{}, err
	}
	return msg, nil
}

func deriveMessageKey(secret, from, to string, seq uint64) []byte {
	sum := sha256.Sum256([]byte(fmt.Sprintf("vx6-ratchet-v1\n%s\n%s\n%s\n%d", secret, from, to, seq)))
	return sum[:]
}

func randomSecret() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func inviteLink(nodeID, nodeName, addr, secret string) string {
	req := friendRequest{
		RequestID: base64.RawURLEncoding.EncodeToString([]byte(nodeID))[:8] + fmt.Sprintf("%d", time.Now().UnixNano()%100000),
		FromID:    nodeID,
		FromName:  nodeName,
		Address:   addr,
		Secret:    secret,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return "vx6chat://invite/" + base64.RawURLEncoding.EncodeToString(marshalJSON(req))
}

func parseInviteLink(link string) (friendRequest, error) {
	const p = "vx6chat://invite/"
	if !strings.HasPrefix(strings.TrimSpace(link), p) {
		return friendRequest{}, fmt.Errorf("invalid invite")
	}
	raw := strings.TrimPrefix(strings.TrimSpace(link), p)
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return friendRequest{}, err
	}
	var req friendRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return friendRequest{}, err
	}
	if req.FromID == "" || req.Address == "" || req.Secret == "" {
		return friendRequest{}, fmt.Errorf("invite missing fields")
	}
	return req, nil
}
