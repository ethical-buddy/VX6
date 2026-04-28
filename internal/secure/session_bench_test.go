package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"net"
	"testing"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/proto"
)

func BenchmarkSecureHandshake(b *testing.B) {
	clientID, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}
	serverID, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		left, right := net.Pipe()
		errCh := make(chan error, 2)

		go func() {
			defer right.Close()
			_, err := Server(right, proto.KindRendezvous, serverID)
			errCh <- err
		}()
		go func() {
			defer left.Close()
			_, err := Client(left, proto.KindRendezvous, clientID)
			errCh <- err
		}()

		for j := 0; j < 2; j++ {
			if err := <-errCh; err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkSecureChunkRoundTrip4K(b *testing.B) {
	key, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		b.Fatal(err)
	}
	aead, err := cipher.NewGCM(key)
	if err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 4096)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sealed := aead.Seal(nil, nonce(0, uint64(i)), payload, nil)
		out, err := aead.Open(nil, nonce(0, uint64(i)), sealed, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) != len(payload) {
			b.Fatalf("unexpected decrypted size %d", len(out))
		}
	}
}

func BenchmarkSecureChunkSeal4K(b *testing.B) {
	key, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		b.Fatal(err)
	}
	aead, err := cipher.NewGCM(key)
	if err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 4096)

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = aead.Seal(nil, nonce(0, uint64(i)), payload, nil)
	}
}

func BenchmarkSecureChunkOpen4K(b *testing.B) {
	key, err := aes.NewCipher(make([]byte, 32))
	if err != nil {
		b.Fatal(err)
	}
	aead, err := cipher.NewGCM(key)
	if err != nil {
		b.Fatal(err)
	}
	payload := make([]byte, 4096)
	sealed := aead.Seal(nil, nonce(0, 0), payload, nil)

	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out, err := aead.Open(nil, nonce(0, 0), sealed, nil)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) != len(payload) {
			b.Fatalf("unexpected decrypted size %d", len(out))
		}
	}
}
