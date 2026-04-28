package onion

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"testing"

	"github.com/vx6/vx6/internal/identity"
)

func BenchmarkOnionCreateClientKey(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, _, err := createClientKey(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOnionSingleHopHandshake(b *testing.B) {
	relayID, err := identity.Generate()
	if err != nil {
		b.Fatal(err)
	}
	relayPub := base64.StdEncoding.EncodeToString(relayID.PublicKey)
	circuitID := circuitIDFromString("bench-single-hop")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		clientPriv, clientPub, err := createClientKey()
		if err != nil {
			b.Fatal(err)
		}
		relayPriv, _, err := createClientKey()
		if err != nil {
			b.Fatal(err)
		}

		created, _, err := buildCreatedPayload(circuitID, clientPub, relayPriv, relayID.PrivateKey)
		if err != nil {
			b.Fatal(err)
		}
		serverPub, err := verifyCreatedPayload(marshalCreatedPayload(created), relayPub, circuitID, clientPub)
		if err != nil {
			b.Fatal(err)
		}
		serverKey, err := ecdh.X25519().NewPublicKey(serverPub[:])
		if err != nil {
			b.Fatal(err)
		}
		shared, err := clientPriv.ECDH(serverKey)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := deriveCircuitKeys(shared, circuitID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOnionLayerWrap3Hop1K(b *testing.B) {
	keyA, err := deriveCircuitKeys([]byte("bench-secret-a"), circuitIDFromString("bench-layer-wrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyB, err := deriveCircuitKeys([]byte("bench-secret-b"), circuitIDFromString("bench-layer-wrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyC, err := deriveCircuitKeys([]byte("bench-secret-c"), circuitIDFromString("bench-layer-wrap"))
	if err != nil {
		b.Fatal(err)
	}
	body := bytes.Repeat([]byte("x"), maxRelayDataPayload)

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		a := *keyA
		bk := *keyB
		c := *keyC
		payload, err := encodeRelayEnvelope(relayCmdData, body)
		if err != nil {
			b.Fatal(err)
		}
		payload = c.sealForward(payload)
		payload = bk.sealForward(payload)
		payload = a.sealForward(payload)
	}
}

func BenchmarkOnionLayerUnwrap3Hop1K(b *testing.B) {
	keyAClient, err := deriveCircuitKeys([]byte("bench-secret-a"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyARelay, err := deriveCircuitKeys([]byte("bench-secret-a"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyBClient, err := deriveCircuitKeys([]byte("bench-secret-b"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyBRelay, err := deriveCircuitKeys([]byte("bench-secret-b"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyCClient, err := deriveCircuitKeys([]byte("bench-secret-c"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	keyCRelay, err := deriveCircuitKeys([]byte("bench-secret-c"), circuitIDFromString("bench-layer-unwrap"))
	if err != nil {
		b.Fatal(err)
	}
	body := bytes.Repeat([]byte("x"), maxRelayDataPayload)

	basePayload, err := encodeRelayEnvelope(relayCmdData, body)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ac := *keyAClient
		ar := *keyARelay
		bc := *keyBClient
		br := *keyBRelay
		cc := *keyCClient
		cr := *keyCRelay

		payload := append([]byte(nil), basePayload...)
		payload = cc.sealForward(payload)
		payload = bc.sealForward(payload)
		payload = ac.sealForward(payload)

		var err error
		payload, err = ar.openForward(payload)
		if err != nil {
			b.Fatal(err)
		}
		payload, err = br.openForward(payload)
		if err != nil {
			b.Fatal(err)
		}
		payload, err = cr.openForward(payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOnionCellReadWrite(b *testing.B) {
	msg := cell{
		Type:      cellTypeRelay,
		CircuitID: circuitIDFromString("bench-cell"),
		Payload:   bytes.Repeat([]byte("y"), cellPayloadSize),
	}

	b.ReportAllocs()
	b.SetBytes(cellSize)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := writeCell(&buf, msg); err != nil {
			b.Fatal(err)
		}
		if _, err := readCell(&buf); err != nil {
			b.Fatal(err)
		}
	}
}
