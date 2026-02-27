//go:build integration

package gateway

import (
	"testing"
)

const testToken = "a099efc7c886bf42878ddf0415691d0e2973637f557689bc072cc27229557c12"
const testGateway = "ws://127.0.0.1:18789"

func TestDeviceIdentity(t *testing.T) {
	dev, err := loadOrCreateDevice()
	if err != nil {
		t.Fatalf("loadOrCreateDevice: %v", err)
	}
	t.Logf("device ID: %s", dev.DeviceID)
	t.Logf("public key: %s", dev.PublicKey[:16]+"...")

	sig, signedAt, err := dev.sign("testnonce", testToken, "operator", []string{"operator.read", "operator.write"})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	t.Logf("signature: %s...", sig[:16])
	t.Logf("signedAt: %d", signedAt)
}

func TestGatewayConnect(t *testing.T) {
	client := New(Options{
		URL:   testGateway,
		Token: testToken,
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
