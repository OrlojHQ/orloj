package a2a

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestSignAndVerifyAgentCardEd25519(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	encoded, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	signer, err := NewPEMCardSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), "test-key")
	if err != nil {
		t.Fatalf("NewPEMCardSigner() error = %v", err)
	}
	card := GenerateAgentCard(
		resources.Agent{Metadata: resources.ObjectMeta{Name: "signed"}, Spec: resources.AgentSpec{Prompt: "sign me"}},
		nil,
		CardGeneratorConfig{PublicBaseURL: "https://example.com", ProtocolVersion: "1.0"},
	)

	signed, err := SignAgentCard(card, signer)
	if err != nil {
		t.Fatalf("SignAgentCard() error = %v", err)
	}
	if len(signed.Signatures) != 1 {
		t.Fatalf("signatures = %d, want 1", len(signed.Signatures))
	}
	if err := VerifyAgentCardSignature(signed, signed.Signatures[0], publicKey); err != nil {
		t.Fatalf("VerifyAgentCardSignature() error = %v", err)
	}

	signed.Description = "tampered"
	if err := VerifyAgentCardSignature(signed, signed.Signatures[0], publicKey); err == nil {
		t.Fatal("expected tampered card signature verification to fail")
	}
}

func TestCardSignatureDoesNotCoverSignatureArray(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	encoded, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	signer, err := NewPEMCardSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), "key-1")
	if err != nil {
		t.Fatalf("NewPEMCardSigner() error = %v", err)
	}
	card := GenerateAgentCard(
		resources.Agent{Metadata: resources.ObjectMeta{Name: "multi"}, Spec: resources.AgentSpec{Prompt: "test"}},
		nil,
		CardGeneratorConfig{PublicBaseURL: "https://example.com"},
	)
	signed, err := SignAgentCard(card, signer)
	if err != nil {
		t.Fatalf("first signature: %v", err)
	}
	first := signed.Signatures[0]
	signed, err = SignAgentCard(signed, signer)
	if err != nil {
		t.Fatalf("second signature: %v", err)
	}
	if err := VerifyAgentCardSignature(signed, first, publicKey); err != nil {
		t.Fatalf("first signature should remain valid after appending: %v", err)
	}
}

func TestNewPEMCardSignerRequiresKeyID(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 key: %v", err)
	}
	encoded, _ := x509.MarshalPKCS8PrivateKey(privateKey)
	_, err = NewPEMCardSigner(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), "")
	if err == nil {
		t.Fatal("expected missing key ID error")
	}
}
