package store

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	defaultPasswordMinLength = 12
	argon2Time               = uint32(3)
	argon2Memory             = uint32(64 * 1024)
	argon2Threads            = uint8(2)
	argon2KeyLen             = uint32(32)
	argon2SaltLen            = 16
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

func ValidatePasswordPolicy(password string, minLen int) error {
	if minLen <= 0 {
		minLen = defaultPasswordMinLength
	}
	if len(password) < minLen {
		return fmt.Errorf("password must be at least %d characters", minLen)
	}
	return nil
}

func GeneratePasswordHash(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is required")
	}
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argon2Memory, argon2Time, argon2Threads, saltB64, hashB64), nil
}

func VerifyPasswordHash(encodedHash, password string) (bool, error) {
	cfg, salt, hash, err := parseArgon2Hash(encodedHash)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, cfg.time, cfg.memory, cfg.threads, uint32(len(hash)))
	if subtle.ConstantTimeCompare(candidate, hash) == 1 {
		return true, nil
	}
	return false, nil
}

type argon2Config struct {
	time    uint32
	memory  uint32
	threads uint8
}

func parseArgon2Hash(encodedHash string) (argon2Config, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}
	if parts[1] != "argon2id" || !strings.HasPrefix(parts[2], "v=") {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}

	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != 19 {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}
	cfg := argon2Config{}
	for _, p := range params {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) != 2 {
			return argon2Config{}, nil, nil, ErrInvalidPasswordHash
		}
		switch kv[0] {
		case "m":
			v, err := strconv.ParseUint(kv[1], 10, 32)
			if err != nil {
				return argon2Config{}, nil, nil, ErrInvalidPasswordHash
			}
			cfg.memory = uint32(v)
		case "t":
			v, err := strconv.ParseUint(kv[1], 10, 32)
			if err != nil {
				return argon2Config{}, nil, nil, ErrInvalidPasswordHash
			}
			cfg.time = uint32(v)
		case "p":
			v, err := strconv.ParseUint(kv[1], 10, 8)
			if err != nil {
				return argon2Config{}, nil, nil, ErrInvalidPasswordHash
			}
			cfg.threads = uint8(v)
		default:
			return argon2Config{}, nil, nil, ErrInvalidPasswordHash
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) == 0 {
		return argon2Config{}, nil, nil, ErrInvalidPasswordHash
	}
	return cfg, salt, hash, nil
}
