package onion

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

const (
	cellVersion          = 1
	cellSize             = 1024
	cellHeaderSize       = 20
	cellPayloadSize      = cellSize - cellHeaderSize
	maxRelayLayers       = 5
	relayTagSize         = 16
	relayEnvelopeSize    = 7
	maxRelayPlainPayload = cellPayloadSize - (maxRelayLayers * relayTagSize)
	maxRelayDataPayload  = maxRelayPlainPayload - relayEnvelopeSize
)

const (
	cellTypeCreate  byte = 1
	cellTypeCreated byte = 2
	cellTypeRelay   byte = 3
	cellTypeDestroy byte = 4
)

const (
	relayCmdExtend    byte = 1
	relayCmdExtended  byte = 2
	relayCmdBegin     byte = 3
	relayCmdConnected byte = 4
	relayCmdData      byte = 5
	relayCmdEnd       byte = 6
	relayCmdError     byte = 7
)

var relayMagic = [4]byte{'V', 'X', 'R', 'L'}

type cell struct {
	Type      byte
	CircuitID [16]byte
	Payload   []byte
}

type createdPayload struct {
	ServerPub [32]byte
	Signature [64]byte
}

type circuitKeyState struct {
	forward         cipher.AEAD
	backward        cipher.AEAD
	forwardCounter  uint64
	backwardCounter uint64
}

func writeCell(w io.Writer, c cell) error {
	if len(c.Payload) > cellPayloadSize {
		return fmt.Errorf("cell payload too large: %d", len(c.Payload))
	}

	var raw [cellSize]byte
	raw[0] = cellVersion
	raw[1] = c.Type
	copy(raw[2:18], c.CircuitID[:])
	binary.BigEndian.PutUint16(raw[18:20], uint16(len(c.Payload)))
	copy(raw[cellHeaderSize:], c.Payload)
	_, err := w.Write(raw[:])
	return err
}

func readCell(r io.Reader) (cell, error) {
	var raw [cellSize]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return cell{}, err
	}
	if raw[0] != cellVersion {
		return cell{}, fmt.Errorf("unsupported onion cell version %d", raw[0])
	}
	length := int(binary.BigEndian.Uint16(raw[18:20]))
	if length < 0 || length > cellPayloadSize {
		return cell{}, fmt.Errorf("invalid onion cell payload length %d", length)
	}
	var circuitID [16]byte
	copy(circuitID[:], raw[2:18])
	payload := append([]byte(nil), raw[cellHeaderSize:cellHeaderSize+length]...)
	return cell{
		Type:      raw[1],
		CircuitID: circuitID,
		Payload:   payload,
	}, nil
}

func randomCircuitID() ([16]byte, string, error) {
	var id [16]byte
	if _, err := io.ReadFull(rand.Reader, id[:]); err != nil {
		return id, "", fmt.Errorf("generate circuit id: %w", err)
	}
	return id, fmt.Sprintf("%x", id[:]), nil
}

func circuitIDFromString(id string) [16]byte {
	if raw, err := hex.DecodeString(id); err == nil && len(raw) == 16 {
		var out [16]byte
		copy(out[:], raw)
		return out
	}
	sum := sha256.Sum256([]byte(id))
	var out [16]byte
	copy(out[:], sum[:16])
	return out
}

func createClientKey() (*ecdh.PrivateKey, [32]byte, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, [32]byte{}, fmt.Errorf("generate onion x25519 key: %w", err)
	}
	var pub [32]byte
	copy(pub[:], priv.PublicKey().Bytes())
	return priv, pub, nil
}

func parseCreatePayload(payload []byte) ([32]byte, error) {
	var pub [32]byte
	if len(payload) != len(pub) {
		return pub, fmt.Errorf("invalid create payload size %d", len(payload))
	}
	copy(pub[:], payload)
	return pub, nil
}

func buildCreatedPayload(circuitID [16]byte, clientPub [32]byte, relayPriv *ecdh.PrivateKey, signer ed25519.PrivateKey) (createdPayload, []byte, error) {
	var out createdPayload
	copy(out.ServerPub[:], relayPriv.PublicKey().Bytes())
	sig := ed25519.Sign(signer, createdSigningPayload(circuitID, clientPub, out.ServerPub))
	copy(out.Signature[:], sig)
	clientKey, err := ecdh.X25519().NewPublicKey(clientPub[:])
	if err != nil {
		return createdPayload{}, nil, fmt.Errorf("parse create public key: %w", err)
	}
	shared, err := relayPriv.ECDH(clientKey)
	if err != nil {
		return createdPayload{}, nil, fmt.Errorf("derive onion shared key: %w", err)
	}
	return out, shared, nil
}

func verifyCreatedPayload(payload []byte, expectedPubKey string, circuitID [16]byte, clientPub [32]byte) ([32]byte, error) {
	var created createdPayload
	if len(payload) != len(created.ServerPub)+len(created.Signature) {
		return [32]byte{}, fmt.Errorf("invalid created payload size %d", len(payload))
	}
	copy(created.ServerPub[:], payload[:32])
	copy(created.Signature[:], payload[32:])
	pubKey, err := base64.StdEncoding.DecodeString(expectedPubKey)
	if err != nil {
		return [32]byte{}, fmt.Errorf("decode relay public key: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return [32]byte{}, fmt.Errorf("relay public key has invalid size")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubKey), createdSigningPayload(circuitID, clientPub, created.ServerPub), created.Signature[:]) {
		return [32]byte{}, fmt.Errorf("created payload signature verification failed")
	}
	return created.ServerPub, nil
}

func marshalCreatedPayload(created createdPayload) []byte {
	out := make([]byte, 0, len(created.ServerPub)+len(created.Signature))
	out = append(out, created.ServerPub[:]...)
	out = append(out, created.Signature[:]...)
	return out
}

func createdSigningPayload(circuitID [16]byte, clientPub, serverPub [32]byte) []byte {
	out := make([]byte, 0, 16+32+32+16)
	out = append(out, []byte("vx6-onion-create\n")...)
	out = append(out, circuitID[:]...)
	out = append(out, clientPub[:]...)
	out = append(out, serverPub[:]...)
	return out
}

func deriveCircuitKeys(shared []byte, circuitID [16]byte) (*circuitKeyState, error) {
	forwardKey, err := deriveKey(shared, circuitID, "forward")
	if err != nil {
		return nil, err
	}
	backwardKey, err := deriveKey(shared, circuitID, "backward")
	if err != nil {
		return nil, err
	}
	forwardAEAD, err := newAEAD(forwardKey)
	if err != nil {
		return nil, err
	}
	backwardAEAD, err := newAEAD(backwardKey)
	if err != nil {
		return nil, err
	}
	return &circuitKeyState{
		forward:  forwardAEAD,
		backward: backwardAEAD,
	}, nil
}

func deriveKey(shared []byte, circuitID [16]byte, label string) ([]byte, error) {
	key, err := hkdf.Key(sha256.New, shared, circuitID[:], "vx6-onion-"+label, 32)
	if err != nil {
		return nil, fmt.Errorf("derive %s key: %w", label, err)
	}
	return key, nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create onion cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create onion gcm: %w", err)
	}
	return aead, nil
}

func (s *circuitKeyState) sealForward(payload []byte) []byte {
	sealed := s.forward.Seal(nil, counterNonce(s.forwardCounter), payload, nil)
	s.forwardCounter++
	return sealed
}

func (s *circuitKeyState) openForward(payload []byte) ([]byte, error) {
	plain, err := s.forward.Open(nil, counterNonce(s.forwardCounter), payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt forward relay payload: %w", err)
	}
	s.forwardCounter++
	return plain, nil
}

func (s *circuitKeyState) sealBackward(payload []byte) []byte {
	sealed := s.backward.Seal(nil, counterNonce(s.backwardCounter), payload, nil)
	s.backwardCounter++
	return sealed
}

func (s *circuitKeyState) openBackward(payload []byte) ([]byte, error) {
	plain, err := s.backward.Open(nil, counterNonce(s.backwardCounter), payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt backward relay payload: %w", err)
	}
	s.backwardCounter++
	return plain, nil
}

func counterNonce(counter uint64) []byte {
	var nonce [12]byte
	binary.BigEndian.PutUint64(nonce[4:], counter)
	return nonce[:]
}

func encodeRelayEnvelope(command byte, body []byte) ([]byte, error) {
	if len(body) > maxRelayDataPayload {
		return nil, fmt.Errorf("relay body too large: %d", len(body))
	}
	out := make([]byte, relayEnvelopeSize+len(body))
	copy(out[:4], relayMagic[:])
	out[4] = command
	binary.BigEndian.PutUint16(out[5:7], uint16(len(body)))
	copy(out[7:], body)
	return out, nil
}

func decodeRelayEnvelope(payload []byte) (command byte, body []byte, recognized bool, err error) {
	if len(payload) < relayEnvelopeSize {
		return 0, nil, false, nil
	}
	if payload[0] != relayMagic[0] || payload[1] != relayMagic[1] || payload[2] != relayMagic[2] || payload[3] != relayMagic[3] {
		return 0, nil, false, nil
	}
	length := int(binary.BigEndian.Uint16(payload[5:7]))
	if length < 0 || relayEnvelopeSize+length > len(payload) {
		return 0, nil, false, fmt.Errorf("invalid relay envelope length %d", length)
	}
	return payload[4], append([]byte(nil), payload[7:7+length]...), true, nil
}
