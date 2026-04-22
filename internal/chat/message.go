package chat

import (
	"time"
	"encoding/json"
)

type Message struct {
	PeerName  string    `json:"peer_name"`
	PeerID    string    `json:"peer_id"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}
func EncodeMessage(msg Message) ([]byte, error) {
	return json.Marshal(msg)
}

func DecodeMessage(data []byte) (Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return msg, err
}
