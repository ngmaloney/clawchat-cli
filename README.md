# clawchat-cli 🦀

Terminal client for [OpenClaw Gateway](https://clawchat.dev). Built with [Go](https://go.dev) + [Charm](https://charm.sh).

![ClawChat CLI](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue)
![Go](https://img.shields.io/badge/go-1.23%2B-00ADD8)
![License](https://img.shields.io/github/license/ngmaloney/clawchat-cli)

## Features

- 🖥️ Full TUI — rounded borders, header bar, streamed responses
- 🔐 SSH tunnel support — connect through a bastion host
- 📜 Message history — loads last 50 messages on connect
- 🔄 Cross-client sync — messages from other clients appear after the assistant responds
- ⌨️ Slash commands — `/help`, `/clear`, `/quit`
- 💾 Config file — `~/.config/clawchat-cli/config.yaml`

## Installation

### Download binary

Grab the latest release from the [releases page](https://github.com/ngmaloney/clawchat-cli/releases).

### go install

```bash
go install github.com/ngmaloney/clawchat-cli/cmd/clawchat-cli@latest
```

## Configuration

On first run, clawchat-cli will tell you where to create the config file:

```
~/.config/clawchat-cli/config.yaml
```

### Direct connection

```yaml
gateway_url: ws://your-gateway-host:18789
token: your-gateway-token
```

### SSH tunnel

```yaml
gateway_url: ws://localhost:18789
token: your-gateway-token

ssh:
  host: your-gateway-host
  port: 22
  user: yourusername
  key: ~/.ssh/id_ed25519
  local_port: 18789
  remote_port: 18789
```

## Usage

```bash
clawchat-cli
```

### Keyboard shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `↑` / `↓` | Scroll chat |
| `PgUp` / `PgDn` | Scroll faster |
| `Ctrl+C` | Quit |

### Slash commands

| Command | Action |
|---------|--------|
| `/help` | Show help |
| `/clear` | Clear chat |
| `/quit` | Quit |

## Build from source

```bash
git clone https://github.com/ngmaloney/clawchat-cli.git
cd clawchat-cli
go build -o clawchat-cli ./cmd/clawchat-cli
```

## Requirements

- Go 1.23+
- An [OpenClaw](https://openclaw.ai) gateway with a valid token

## Related

- [ClawChat](https://github.com/ngmaloney/clawchat) — Desktop client (Electron)
- [clawchat.dev](https://clawchat.dev)
