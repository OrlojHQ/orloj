package resources

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"strings"
)

type SealingKeyMaterial struct {
	KeyID        string
	PublicKeyPEM string
	PrivateKeyPEM string
	PublicKey    *rsa.PublicKey
	PrivateKey   *rsa.PrivateKey
}

func GenerateSealingKeyMaterial() (SealingKeyMaterial, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return SealingKeyMaterial{}, fmt.Errorf("generate sealing key: %w", err)
	}
	publicKey := &privateKey.PublicKey

	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return SealingKeyMaterial{}, fmt.Errorf("marshal sealing public key: %w", err)
	}
	privateDER := x509.MarshalPKCS1PrivateKey(privateKey)
	keyID := sealingKeyID(publicDER)

	return SealingKeyMaterial{
		KeyID:         keyID,
		PublicKeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})),
		PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateDER})),
		PublicKey:     publicKey,
		PrivateKey:    privateKey,
	}, nil
}

func ParseSealingPublicKeyPEM(raw string) (*rsa.PublicKey, string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, "", fmt.Errorf("decode sealing public key PEM: no PEM block found")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse sealing public key: %w", err)
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, "", fmt.Errorf("parse sealing public key: expected RSA public key")
	}
	return pub, sealingKeyID(block.Bytes), nil
}

func ParseSealingPrivateKeyPEM(raw string) (*rsa.PrivateKey, string, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(raw)))
	if block == nil {
		return nil, "", fmt.Errorf("decode sealing private key PEM: no PEM block found")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse sealing private key: %w", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal sealing public key: %w", err)
	}
	return privateKey, sealingKeyID(publicDER), nil
}

func SealSecret(secret Secret, keyID string, publicKey *rsa.PublicKey) (SealedSecret, error) {
	if publicKey == nil {
		return SealedSecret{}, fmt.Errorf("sealing public key is required")
	}
	if err := secret.Normalize(); err != nil {
		return SealedSecret{}, err
	}
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		pubDER, err := x509.MarshalPKIXPublicKey(publicKey)
		if err != nil {
			return SealedSecret{}, fmt.Errorf("marshal sealing public key: %w", err)
		}
		keyID = sealingKeyID(pubDER)
	}
	out := SealedSecret{
		APIVersion: secret.APIVersion,
		Kind:       "SealedSecret",
		Metadata: ObjectMeta{
			Name:        secret.Metadata.Name,
			Namespace:   secret.Metadata.Namespace,
			Labels:      copyStringMap(secret.Metadata.Labels),
			Annotations: copyStringMap(secret.Metadata.Annotations),
		},
		Spec: SealedSecretSpec{
			EncryptedData: make(map[string]SealedValue, len(secret.Spec.Data)),
			Template: SealedSecretTemplateSecret{
				Labels:      copyStringMap(secret.Metadata.Labels),
				Annotations: copyStringMap(secret.Metadata.Annotations),
			},
		},
	}

	for entryKey, value := range secret.Spec.Data {
		sealedValue, err := sealSecretValue(secret.Metadata.Namespace, secret.Metadata.Name, entryKey, value, keyID, publicKey)
		if err != nil {
			return SealedSecret{}, err
		}
		out.Spec.EncryptedData[entryKey] = sealedValue
	}
	if err := out.Normalize(); err != nil {
		return SealedSecret{}, err
	}
	return out, nil
}

func UnsealSealedSecret(item SealedSecret, expectedKeyID string, privateKey *rsa.PrivateKey) (Secret, error) {
	if privateKey == nil {
		return Secret{}, fmt.Errorf("sealing private key is required")
	}
	if err := item.Normalize(); err != nil {
		return Secret{}, err
	}
	data := make(map[string]string, len(item.Spec.EncryptedData))
	for entryKey, sealedValue := range item.Spec.EncryptedData {
		if expectedKeyID != "" && sealedValue.KeyID != expectedKeyID {
			return Secret{}, fmt.Errorf("spec.encryptedData.%s.keyId %q does not match active key %q", entryKey, sealedValue.KeyID, expectedKeyID)
		}
		plaintext, err := unsealSecretValue(item.Metadata.Namespace, item.Metadata.Name, entryKey, sealedValue, privateKey)
		if err != nil {
			return Secret{}, err
		}
		data[entryKey] = plaintext
	}
	secret := Secret{
		APIVersion: "orloj.dev/v1",
		Kind:       "Secret",
		Metadata: ObjectMeta{
			Name:        item.Metadata.Name,
			Namespace:   item.Metadata.Namespace,
			Labels:      copyStringMap(item.Spec.Template.Labels),
			Annotations: copyStringMap(item.Spec.Template.Annotations),
		},
		Spec: SecretSpec{
			Data: data,
		},
	}
	if err := secret.Normalize(); err != nil {
		return Secret{}, err
	}
	return secret, nil
}

func sealSecretValue(namespace, name, entryKey, value, keyID string, publicKey *rsa.PublicKey) (SealedValue, error) {
	dataKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dataKey); err != nil {
		return SealedValue{}, fmt.Errorf("generate data key for %q: %w", entryKey, err)
	}
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return SealedValue{}, fmt.Errorf("encrypt %q: %w", entryKey, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return SealedValue{}, fmt.Errorf("encrypt %q: %w", entryKey, err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return SealedValue{}, fmt.Errorf("generate nonce for %q: %w", entryKey, err)
	}
	aad := sealedSecretAAD(namespace, name, entryKey)
	ciphertext := gcm.Seal(nonce, nonce, []byte(value), aad)
	wrappedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, dataKey, nil)
	if err != nil {
		return SealedValue{}, fmt.Errorf("wrap data key for %q: %w", entryKey, err)
	}
	return SealedValue{
		KeyID:      keyID,
		WrappedKey: base64.StdEncoding.EncodeToString(wrappedKey),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, nil
}

func unsealSecretValue(namespace, name, entryKey string, value SealedValue, privateKey *rsa.PrivateKey) (string, error) {
	wrappedKey, err := base64.StdEncoding.DecodeString(value.WrappedKey)
	if err != nil {
		return "", fmt.Errorf("spec.encryptedData.%s.wrappedKey invalid base64: %w", entryKey, err)
	}
	dataKey, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, wrappedKey, nil)
	if err != nil {
		return "", fmt.Errorf("unwrap data key for %q: %w", entryKey, err)
	}
	rawCiphertext, err := base64.StdEncoding.DecodeString(value.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("spec.encryptedData.%s.ciphertext invalid base64: %w", entryKey, err)
	}
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", fmt.Errorf("decrypt %q: %w", entryKey, err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt %q: %w", entryKey, err)
	}
	if len(rawCiphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("decrypt %q: ciphertext too short", entryKey)
	}
	nonce, ciphertext := rawCiphertext[:gcm.NonceSize()], rawCiphertext[gcm.NonceSize():]
	aad := sealedSecretAAD(namespace, name, entryKey)
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		legacyAAD := legacySealedSecretAAD(namespace, name, entryKey)
		plaintext, err = gcm.Open(nil, nonce, ciphertext, legacyAAD)
		if err != nil {
			return "", fmt.Errorf("decrypt %q: %w", entryKey, err)
		}
	}
	return string(plaintext), nil
}

const sealedSecretAADVersion = "AES-256-GCM-v1"

func sealedSecretAAD(namespace, name, entryKey string) []byte {
	return []byte(sealedSecretAADVersion + "\x00" +
		NormalizeNamespace(namespace) + "\x00" +
		strings.TrimSpace(name) + "\x00" +
		strings.TrimSpace(entryKey))
}

func legacySealedSecretAAD(namespace, name, entryKey string) []byte {
	return []byte(NormalizeNamespace(namespace) + "\x00" + strings.TrimSpace(name) + "\x00" + strings.TrimSpace(entryKey))
}

func sealingKeyID(publicDER []byte) string {
	sum := sha256.Sum256(publicDER)
	return hex.EncodeToString(sum[:16])
}
