# ClawChat CLI — PRD Questions

Questions to review before writing the PRD.

---

## 1. Users & Use Cases

1. Who is the primary user? (developers, power users, server admins, all of the above?)
2. What's the core motivation for a CLI version — scripting, SSH sessions, terminal-first workflow, or something else?
3. Should it work headlessly (piped output, no TUI) or is interactive TUI always required?

---

## 2. Scope vs ClawChat Desktop

4. Is this a full feature-parity replacement for ClawChat, or a focused subset?
5. Which features are **must-have** at launch vs nice-to-have:
   - Session switching
   - Message history/scrollback
   - File/image attachments
   - Markdown rendering
   - Notifications (desktop or terminal bell)
   - Multi-gateway support
6. Are there things the CLI should do that the desktop app *can't* (scripting, piping, automation)?

---

## 3. Auth & Configuration

7. Same connection model as ClawChat (gateway URL + token)? Or something different?
8. Where does config live — `~/.config/clawchat-cli/config.yaml`, env vars, flags, or all three?
9. Should it support multiple saved gateway profiles (like SSH config)?

---

## 4. UX & Interface

10. Full TUI (Bubble Tea alt-screen) or inline terminal output (like a chat log that scrolls)?
11. Key bindings — vim-style, emacs-style, or custom?
12. Should sessions/channels be shown in a sidebar pane (split layout) or navigated via commands?
13. Any specific Charm libraries already in mind (Wish for SSH hosting, Huh for forms, Glamour for markdown)?

---

## 5. Distribution

14. Target platforms: macOS only, or Linux + Windows too?
15. Distribution method: `brew install`, `go install`, GitHub releases binary, or all of the above?
16. Should there be a `clawchat-cli update` self-update command?

---

## 6. Timeline & Priority

17. Is this a personal tool or something that will be publicly marketed alongside ClawChat?
18. What does "done enough to ship" look like — MVP feature set?
19. Any hard deadline or is this exploratory for now?
