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
	NodeID      string   `json:"node_id"`
	NodeName    string   `json:"node_name"`
	ServiceName string   `json:"service_name"`
	Address     string   `json:"address,omitempty"`      // Empty if hidden
	IsHidden    bool     `json:"is_hidden"`               // If true, IP is masked
	IntroPoints []string `json:"intro_points,omitempty"` // Relay IDs to reach this service
	PublicKey   string   `json:"public_key"`
	IssuedAt    string   `json:"issued_at"`
	ExpiresAt   string   `json:"expires_at"`
	Signature   string   `json:"signature"`
}

func NewServiceRecord(id identity.Identity, nodeName, serviceName, address string, ttl time.Duration, now time.Time) (ServiceRecord, error) {
	if nodeName == "" {
		return ServiceRecord{}, fmt.Errorf("node name cannot be empty")
	}
	if err := ValidateServiceName(serviceName); err != nil {
		return ServiceRecord{}, err
	}
	// Address is only required if NOT hidden. We validate it later during signature if present.
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
	
	return rec, nil
}

func SignServiceRecord(id identity.Identity, rec *ServiceRecord) error {
	sig := ed25519.Sign(id.PrivateKey, serviceSigningPayload(*rec))
	rec.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

func VerifyServiceRecord(rec ServiceRecord, now time.Time) error {
	if rec.NodeID == "" || rec.NodeName == "" || rec.ServiceName == "" {
		return fmt.Errorf("service record missing required fields")
	}
	if !rec.IsHidden && rec.Address == "" {
		return fmt.Errorf("non-hidden service requires an address")
	}
	if !rec.IsHidden {
		if err := transfer.ValidateIPv6Address(rec.Address); err != nil {
			return err
		}
	}

	publicKey, err := base64.StdEncoding.DecodeString(rec.PublicKey)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	signature, err := base64.StdEncoding.DecodeString(rec.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	if _, err := time.Parse(time.RFC3339, rec.IssuedAt); err != nil {
		return fmt.Errorf("parse issued_at: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, rec.ExpiresAt)
	if err != nil {
		return fmt.Errorf("parse expires_at: %w", err)
	}
	if now.UTC().After(expiresAt) {
		return fmt.Errorf("service record has expired")
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
	// We include the IsHidden and IntroPoints in the signed payload to prevent IP spoofing
	intros := strings.Join(rec.IntroPoints, ",")
	hiddenStr := "false"
	if rec.IsHidden {
		hiddenStr = "true"
	}

	return []byte(
		rec.NodeID + "\n" +
			rec.NodeName + "\n" +
			rec.ServiceName + "\n" +
			rec.Address + "\n" +
			hiddenStr + "\n" +
			intros + "\n" +
			rec.PublicKey + "\n" +
			rec.IssuedAt + "\n" +
			rec.ExpiresAt + "\n",
	)
}
