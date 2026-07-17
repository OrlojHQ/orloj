package a2a

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

// CardSigner signs the RFC 8785 canonical representation of an Agent Card
// using the detached-payload JWS form defined by the A2A specification.
type CardSigner interface {
	Sign(card AgentCard) (AgentCardSignature, error)
}

type jwsCardSigner struct {
	key       crypto.Signer
	algorithm string
	keyID     string
}

// LoadPEMCardSigner loads an RSA, P-256 ECDSA, or Ed25519 private key.
func LoadPEMCardSigner(path, keyID string) (CardSigner, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read A2A card signing key: %w", err)
	}
	return NewPEMCardSigner(data, keyID)
}

// NewPEMCardSigner creates a signer from PKCS#8, PKCS#1, or SEC1 PEM data.
func NewPEMCardSigner(data []byte, keyID string) (CardSigner, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("A2A card signing key is not PEM encoded")
	}

	var parsed any
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		parsed, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported A2A card signing PEM type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("parse A2A card signing key: %w", err)
	}

	signer, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("unsupported A2A card signing key type %T", parsed)
	}
	algorithm, err := signerAlgorithm(signer)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(keyID) == "" {
		return nil, errors.New("A2A card signing key ID is required")
	}
	return &jwsCardSigner{key: signer, algorithm: algorithm, keyID: strings.TrimSpace(keyID)}, nil
}

// SignAgentCard returns a card with a newly appended signature. Existing
// signatures are excluded from the signed payload, as required by A2A.
func SignAgentCard(card AgentCard, signer CardSigner) (AgentCard, error) {
	if signer == nil {
		return card, nil
	}
	signature, err := signer.Sign(card)
	if err != nil {
		return AgentCard{}, err
	}
	card.Signatures = append(card.Signatures, signature)
	return card, nil
}

func (s *jwsCardSigner) Sign(card AgentCard) (AgentCardSignature, error) {
	protectedJSON, err := json.Marshal(struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
		Type      string `json:"typ"`
	}{
		Algorithm: s.algorithm,
		KeyID:     s.keyID,
		Type:      "agent-card+jws",
	})
	if err != nil {
		return AgentCardSignature{}, err
	}
	protected := base64.RawURLEncoding.EncodeToString(protectedJSON)
	payload, err := canonicalAgentCard(card)
	if err != nil {
		return AgentCardSignature{}, err
	}
	signingInput := []byte(protected + "." + base64.RawURLEncoding.EncodeToString(payload))
	signature, err := signJWS(s.key, s.algorithm, signingInput)
	if err != nil {
		return AgentCardSignature{}, err
	}
	return AgentCardSignature{
		Protected: protected,
		Signature: base64.RawURLEncoding.EncodeToString(signature),
	}, nil
}

// VerifyAgentCardSignature verifies one signature against a public key.
func VerifyAgentCardSignature(card AgentCard, signature AgentCardSignature, publicKey crypto.PublicKey) error {
	protectedJSON, err := base64.RawURLEncoding.DecodeString(signature.Protected)
	if err != nil {
		return fmt.Errorf("decode protected JWS header: %w", err)
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(protectedJSON, &header); err != nil {
		return fmt.Errorf("decode protected JWS header: %w", err)
	}
	payload, err := canonicalAgentCard(card)
	if err != nil {
		return err
	}
	signingInput := []byte(signature.Protected + "." + base64.RawURLEncoding.EncodeToString(payload))
	rawSignature, err := base64.RawURLEncoding.DecodeString(signature.Signature)
	if err != nil {
		return fmt.Errorf("decode JWS signature: %w", err)
	}
	return verifyJWS(publicKey, header.Algorithm, signingInput, rawSignature)
}

func canonicalAgentCard(card AgentCard) ([]byte, error) {
	card.Signatures = nil
	raw, err := json.Marshal(card)
	if err != nil {
		return nil, fmt.Errorf("marshal Agent Card for signing: %w", err)
	}
	canonical, err := jsoncanonicalizer.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Agent Card: %w", err)
	}
	return canonical, nil
}

func signerAlgorithm(signer crypto.Signer) (string, error) {
	switch key := signer.(type) {
	case *rsa.PrivateKey:
		if key.N.BitLen() < 2048 {
			return "", errors.New("A2A card RSA signing key must be at least 2048 bits")
		}
		return "RS256", nil
	case *ecdsa.PrivateKey:
		if key.Curve != elliptic.P256() {
			return "", errors.New("A2A card ECDSA signing key must use P-256")
		}
		return "ES256", nil
	case ed25519.PrivateKey:
		return "EdDSA", nil
	default:
		return "", fmt.Errorf("unsupported A2A card signing key type %T", signer)
	}
}

func signJWS(key crypto.Signer, algorithm string, signingInput []byte) ([]byte, error) {
	switch algorithm {
	case "RS256":
		digest := sha256.Sum256(signingInput)
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("RS256 requires an RSA key")
		}
		return rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	case "ES256":
		digest := sha256.Sum256(signingInput)
		ecdsaKey, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("ES256 requires an ECDSA key")
		}
		r, s, err := ecdsa.Sign(rand.Reader, ecdsaKey, digest[:])
		if err != nil {
			return nil, err
		}
		size := (ecdsaKey.Curve.Params().BitSize + 7) / 8
		signature := make([]byte, size*2)
		r.FillBytes(signature[:size])
		s.FillBytes(signature[size:])
		return signature, nil
	case "EdDSA":
		edKey, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, errors.New("EdDSA requires an Ed25519 key")
		}
		return ed25519.Sign(edKey, signingInput), nil
	default:
		return nil, fmt.Errorf("unsupported JWS algorithm %q", algorithm)
	}
}

func verifyJWS(publicKey crypto.PublicKey, algorithm string, signingInput, signature []byte) error {
	switch algorithm {
	case "RS256":
		key, ok := publicKey.(*rsa.PublicKey)
		if !ok {
			return errors.New("RS256 signature requires an RSA public key")
		}
		digest := sha256.Sum256(signingInput)
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature)
	case "ES256":
		key, ok := publicKey.(*ecdsa.PublicKey)
		if !ok || key.Curve != elliptic.P256() {
			return errors.New("ES256 signature requires a P-256 public key")
		}
		size := (key.Curve.Params().BitSize + 7) / 8
		if len(signature) != size*2 {
			return errors.New("invalid ES256 signature length")
		}
		r := new(big.Int).SetBytes(signature[:size])
		s := new(big.Int).SetBytes(signature[size:])
		digest := sha256.Sum256(signingInput)
		if !ecdsa.Verify(key, digest[:], r, s) {
			return errors.New("invalid ES256 signature")
		}
		return nil
	case "EdDSA":
		key, ok := publicKey.(ed25519.PublicKey)
		if !ok {
			return errors.New("EdDSA signature requires an Ed25519 public key")
		}
		if !ed25519.Verify(key, signingInput, signature) {
			return errors.New("invalid EdDSA signature")
		}
		return nil
	default:
		return fmt.Errorf("unsupported JWS algorithm %q", algorithm)
	}
}
