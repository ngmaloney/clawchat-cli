// Package tunnel manages an SSH port-forward tunnel using the system ssh binary.
package tunnel

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/ngmaloney/clawchat-cli/internal/config"
)

// Tunnel wraps a spawned ssh process providing a local port forward.
type Tunnel struct {
	LocalPort int
	proc      *exec.Cmd
}

// Start establishes the SSH tunnel and returns the local port to connect to.
// It blocks until the tunnel is ready (local port accepts connections) or fails.
func Start(cfg *config.SSH) (*Tunnel, error) {
	localPort, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("finding free port: %w", err)
	}

	keyPath := config.ExpandTilde(cfg.KeyPath)
	remotePort := cfg.RemotePort
	if remotePort == 0 {
		remotePort = 18789
	}
	sshPort := cfg.Port
	if sshPort == 0 {
		sshPort = 22
	}

	args := []string{
		"-N",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=30",
		"-o", "BatchMode=yes",
		"-L", fmt.Sprintf("%d:127.0.0.1:%d", localPort, remotePort),
		"-p", fmt.Sprintf("%d", sshPort),
	}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, fmt.Sprintf("%s@%s", cfg.User, cfg.Host))

	proc := exec.Command("ssh", args...)

	var stderr strings.Builder
	proc.Stderr = &stderr

	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("starting ssh: %w", err)
	}

	t := &Tunnel{LocalPort: localPort, proc: proc}

	// Poll until the local port is accepting connections.
	if err := t.waitReady(15 * time.Second); err != nil {
		_ = proc.Process.Kill()
		return nil, fmt.Errorf("tunnel did not become ready: %w (ssh stderr: %s)", err, stderr.String())
	}

	return t, nil
}

// Stop kills the SSH tunnel process.
func (t *Tunnel) Stop() {
	if t.proc != nil && t.proc.Process != nil {
		_ = t.proc.Process.Kill()
		_ = t.proc.Wait()
	}
}

// GatewayURL returns the local WebSocket URL to connect through the tunnel.
func (t *Tunnel) GatewayURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d", t.LocalPort)
}

// waitReady polls the local port until it accepts a TCP connection.
func (t *Tunnel) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if the process died early
		if t.proc.ProcessState != nil {
			return fmt.Errorf("ssh process exited prematurely")
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", t.LocalPort), 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s", timeout)
}

// freePort finds an available local TCP port.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port, nil
}
