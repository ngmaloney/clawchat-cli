//go:build integration

package gateway

import (
	"os"
	"testing"
)

func getTestConfig(t *testing.T) (gatewayURL, token string) {
	t.Helper()
	token = os.Getenv("CLAWCHAT_TEST_TOKEN")
	gatewayURL = os.Getenv("CLAWCHAT_TEST_GATEWAY")
	if gatewayURL == "" {
		gatewayURL = "ws://127.0.0.1:18789"
	}
	if token == "" {
		t.Skip("CLAWCHAT_TEST_TOKEN not set — skipping integration test")
	}
	return
}

func TestDeviceIdentity(t *testing.T) {
	_, token := getTestConfig(t)

	dev, err := loadOrCreateDevice()
	if err != nil {
		t.Fatalf("loadOrCreateDevice: %v", err)
	}
	t.Logf("device ID: %s", dev.DeviceID)
	t.Logf("public key: %s", dev.PublicKey[:16]+"...")

	sig, signedAt, err := dev.sign("testnonce", token, "operator", []string{"operator.read", "operator.write"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	t.Logf("signature: %s...", sig[:16])
	t.Logf("signedAt: %d", signedAt)
}

func TestGatewayConnect(t *testing.T) {
	gatewayURL, token := getTestConfig(t)

	client := New(Options{
		URL:   gatewayURL,
		Token: token,
	})

	if err := client.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	t.Logf("connected, status: %s", client.Status())

	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	t.Logf("sessions: %d", len(sessions))
	for _, s := range sessions {
		t.Logf("  - %s (%s)", s.Key, s.Model)
	}
}
