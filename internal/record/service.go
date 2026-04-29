package record

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/vx6/vx6/internal/identity"
	"github.com/vx6/vx6/internal/transfer"
)

type ServiceRecord struct {
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	ServiceName string `json:"service_name"`
	Address     string `json:"address"`
	PublicKey   string `json:"public_key"`
	IssuedAt    string `json:"issued_at"`
	ExpiresAt   string `json:"expires_at"`
	Signature   string `json:"signature"`
}

func NewServiceRecord(id identity.Identity, nodeName, serviceName, address string, ttl time.Duration, now time.Time) (ServiceRecord, error) {
	if nodeName == "" {
		return ServiceRecord{}, fmt.Errorf("node name cannot be empty")
	}
	if err := ValidateServiceName(serviceName); err != nil {
		return ServiceRecord{}, err
	}
	if err := transfer.ValidateIPv6Address(address); err != nil {
		return ServiceRecord{}, err
	}
	if ttl <= 0 {
		return ServiceRecord{}, fmt.Errorf("ttl must be greater than zero")
	}
	if err := id.Validate(); err != nil {
		return ServiceRecord{}, err
	}

	rec := ServiceRecord{
		NodeID:      id.NodeID,
		NodeName:    nodeName,
		ServiceName: serviceName,
		Address:     address,
		PublicKey:   base64.StdEncoding.EncodeToString(id.PublicKey),
		IssuedAt:    now.UTC().Format(time.RFC3339),
		ExpiresAt:   now.UTC().Add(ttl).Format(time.RFC3339),
	}
	rec.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(id.PrivateKey, serviceSigningPayload(rec)))
	return rec, nil
}

func VerifyServiceRecord(rec ServiceRecord, now time.Time) error {
	if rec.NodeID == "" || rec.NodeName == "" || rec.ServiceName == "" || rec.Address == "" {
		return fmt.Errorf("service record missing required fields")
	}
	if err := ValidateServiceName(rec.ServiceName); err != nil {
		return err
	}
	if err := transfer.ValidateIPv6Address(rec.Address); err != nil {
		return err
	}

	publicKey, err := base64.StdEncoding.DecodeString(rec.PublicKey)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(rec.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("service record contains invalid public key")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("service record contains invalid signature")
	}

	issuedAt, err := time.Parse(time.RFC3339, rec.IssuedAt)
	if err != nil {
		return fmt.Errorf("parse issued_at: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err != nil {
		return fmt.Errorf("parse expires_at: %w", err)
	}
	if !expiresAt.After(issuedAt) {
		return fmt.Errorf("service record expiry must be after issue time")
	}
	if now.UTC().After(expiresAt) {
		return fmt.Errorf("service record has expired")
	}
	if want := identity.NodeIDFromPublicKey(ed25519.PublicKey(publicKey)); want != rec.NodeID {
		return fmt.Errorf("service record node id does not match public key")
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), serviceSigningPayload(rec), signature) {
		return fmt.Errorf("service record signature verification failed")
	}

	return nil
}

func FullServiceName(nodeName, serviceName string) string {
	return nodeName + "." + serviceName
}

func ValidateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if strings.Contains(name, ".") {
		return fmt.Errorf("service name %q cannot contain dots", name)
	}
	return nil
}

func serviceSigningPayload(rec ServiceRecord) []byte {
	return []byte(
		rec.NodeID + "\n" +
			rec.NodeName + "\n" +
			rec.ServiceName + "\n" +
			rec.Address + "\n" +
			rec.PublicKey + "\n" +
			rec.IssuedAt + "\n" +
			rec.ExpiresAt + "\n",
	)
}
