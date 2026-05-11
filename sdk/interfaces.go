package sdk

import "context"

// Transport defines the minimal runtime data-channel behavior needed by apps.
// Implementations may be Go-native or foreign-language adapters.
type Transport interface {
	Dial(ctx context.Context, addr string) (Conn, error)
}

// Conn is a generic connection contract for transport adapters.
type Conn interface {
	Write([]byte) (int, error)
	Read([]byte) (int, error)
	Close() error
}

// Discovery defines peer/service lookup contracts.
type Discovery interface {
	LookupNode(ctx context.Context, name string) ([]byte, error)
	LookupService(ctx context.Context, name string) ([]byte, error)
}

// Signaling defines signaling envelopes for call/session negotiation.
type Signaling interface {
	Publish(ctx context.Context, key string, payload []byte) error
	Fetch(ctx context.Context, key string) ([]byte, error)
}

// SessionCrypto defines session key schedule and envelope protection APIs.
type SessionCrypto interface {
	Seal(ctx context.Context, peerID string, msg []byte) ([]byte, error)
	Open(ctx context.Context, peerID string, payload []byte) ([]byte, error)
}

