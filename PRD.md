# ClawChat CLI — Product Requirements Document

**Version:** 0.1 (Draft)
**Stack:** Go + Charm (Bubble Tea)
**Repo:** github.com/ngmaloney/clawchat-cli

---

## Overview

ClawChat CLI is a terminal-based client for OpenClaw gateways. It is a sister project to [ClawChat](https://clawchat.dev) — purpose-built for users who live in the terminal or connect to their gateway over SSH, where a desktop Electron app is impractical or unavailable.

---

## Target User

Developers and power users who:
- Work primarily in the terminal
- Access their OpenClaw gateway over SSH
- Prefer keyboard-driven interfaces

---

## Goals

- Provide a fast, minimal TUI for chatting with an OpenClaw gateway
- Match the ClawChat brand as a sister project
- Ship a clean v1 with a tight feature set; no scope creep

---

## Non-Goals (v1)

- File or image attachments
- Session management / switching
- Desktop notifications
- Headless / scriptable mode
- Node mode (camera, screen, system commands)
- Multiple gateway profiles

---

## Features — v1

### 1. Connection
- Connect to a single OpenClaw gateway via WebSocket (URL + token)
- Credentials persisted locally (config file), auto-connect on launch
- Auto-reconnect with exponential backoff on disconnect

### 2. Chat
- Send messages and receive streamed responses with live rendering
- Full message history with scrollback (`PgUp` / `PgDn`)
- Markdown rendering — code blocks with syntax highlighting, bold, italic, links, lists (via Glamour)

### 3. Slash Commands
- `/new` — start a new session
- `/model` — switch model
- `/thinking` — toggle thinking mode
- `/status` — show session status
- Additional commands as supported by the gateway

### 4. Status Bar
- Always-visible bottom bar showing: connection state, current model, gateway URL

---

## Configuration

Priority order: CLI flags → environment variables → config file

**Config file:** `~/.config/clawchat-cli/config.yaml`

```yaml
gateway_url: ws://localhost:18789
token: your-token-here
```

**Environment variables:**
- `CLAWCHAT_GATEWAY_URL`
- `CLAWCHAT_TOKEN`

**CLI flags:**
- `--gateway-url`
- `--token`

---

## UX & Key Bindings

Follows Charm/Bubble Tea conventions:

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `PgUp` / `PgDn` | Scroll message history |
| `Esc` | Cancel / clear input |
| `Ctrl+C` | Quit |

No vim bindings. No sidebar.

---

## Distribution — v1

- **GitHub Releases** — pre-built binaries for macOS (arm64 + amd64), Linux (amd64), Windows (amd64)
- **`go install`** — `go install github.com/ngmaloney/clawchat-cli@latest`

---

## Positioning

ClawChat CLI is a **sister project to ClawChat** — same gateway, different surface. Together they cover:

| | ClawChat | ClawChat CLI |
|---|---|---|
| Surface | Desktop (Electron) | Terminal |
| Best for | GUI users, Windows/Linux desktop | SSH, terminal-first, remote servers |
| Platforms | macOS, Windows, Linux | macOS, Linux, Windows |

---

## Open Questions

- Should the config file also support a `name` or `profile` field for future multi-gateway support?
- Versioning/release cadence relative to ClawChat?
- Shared branding assets (icon for GitHub, etc.)?
