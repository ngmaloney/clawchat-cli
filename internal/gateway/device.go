package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// deviceIdentity holds the persistent ed25519 keypair for this CLI installation.
type deviceIdentity struct {
	Version    int    `json:"version"`
	DeviceID   string `json:"deviceId"`
	PublicKey  string `json:"publicKey"`  // base64url
	PrivateKey string `json:"privateKey"` // base64url
	CreatedAt  int64  `json:"createdAtMs"`
}

// deviceKeyPath returns the path to the stored device identity file.
func deviceKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clawchat-cli", "device.json")
}

// loadOrCreateDevice loads the device identity from disk, creating it if needed.
func loadOrCreateDevice() (*deviceIdentity, error) {
	path := deviceKeyPath()

	// Try loading existing identity
	if data, err := os.ReadFile(path); err == nil {
		var id deviceIdentity
		if err := json.Unmarshal(data, &id); err == nil && id.Version == 1 && id.DeviceID != "" {
			// Verify device ID matches public key
			pubBytes, err := base64URLDecode(id.PublicKey)
			if err == nil {
				computed := deviceIDFromPubKey(pubBytes)
				if computed == id.DeviceID {
					return &id, nil
				}
			}
		}
	}

	// Generate new keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key pair: %w", err)
	}

	id := &deviceIdentity{
		Version:    1,
		DeviceID:   deviceIDFromPubKey(pubKey),
		PublicKey:  base64URLEncode(pubKey),
		PrivateKey: base64URLEncode(privKey),
		CreatedAt:  time.Now().UnixMilli(),
	}

	// Persist
	if err := os.MkdirAll(filepath.Dir(path), 0700); err == nil {
		if data, err := json.Marshal(id); err == nil {
			_ = os.WriteFile(path, data, 0600)
		}
	}

	return id, nil
}

// sign signs the challenge nonce with the device private key.
// Signature payload format matches ClawChat's device-crypto-ed25519.ts:
//
//	v2|{deviceId}|{clientId}|{clientMode}|{role}|{scopes}|{signedAtMs}|{token}|{nonce}
func (id *deviceIdentity) sign(nonce, token, role string, scopes []string) (string, int64, error) {
	privBytes, err := base64URLDecode(id.PrivateKey)
	if err != nil {
		return "", 0, fmt.Errorf("decoding private key: %w", err)
	}
	privKey := ed25519.PrivateKey(privBytes)

	signedAtMs := time.Now().UnixMilli()
	scopesStr := strings.Join(scopes, ",")

	payload := strings.Join([]string{
		"v2",
		id.DeviceID,
		"cli",     // clientId
		"cli",     // clientMode
		role,
		scopesStr,
		fmt.Sprintf("%d", signedAtMs),
		token,
		nonce,
	}, "|")

	sig := ed25519.Sign(privKey, []byte(payload))
	return base64URLEncode(sig), signedAtMs, nil
}

// deviceIDFromPubKey computes the device ID as a hex SHA-256 of the public key.
func deviceIDFromPubKey(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	return fmt.Sprintf("%x", hash)
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func base64URLDecode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
