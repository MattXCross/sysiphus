# sysiphus

<img width="1907" height="1053" alt="image" src="https://github.com/user-attachments/assets/a17a57b3-2296-4ed5-a5b5-318ac75b2a68" />

`sysiphus` is a Go + Bubble Tea terminal coding agent that uses GitHub Copilot's Go SDK as its runtime.

## Current MVP

- Bubble Tea TUI with transcript, activity feed, runtime status, and session list
- Connects to the local GitHub Copilot CLI runtime through `github.com/github/copilot-sdk/go`
- Streams assistant output into the transcript pane
- Supports `build` and `plan` agent presets for new sessions
- Supports new session and resume-latest session flows
- Shows permission requests and outcomes in the activity feed
- Exposes an allow-all default policy and a conservative toggle

## Requirements

- Go 1.25+
- GitHub Copilot CLI installed and authenticated as `copilot`

This machine did not have `copilot` installed while this MVP was built, so the UI handles that case and reports it clearly.

## Run

```bash
go run ./cmd/sysiphus
```

## Install Globally

Install `sysiphus` into `~/.local/bin`:

```bash
./scripts/install.sh
```

Install to a custom directory:

```bash
./scripts/install.sh /usr/local/bin
```

After installation, run from anywhere with:

```bash
sysiphus
```

## Keys

- `Enter`: send prompt
- `Tab`: swap between `build` and `plan` and start a fresh session in that mode
- `Up` / `Down`: scroll transcript by line
- `PgUp` / `PgDn`: scroll transcript
- `Ctrl+Up` / `Ctrl+Down`: scroll transcript by line
- `Alt+PgUp` / `Alt+PgDn`: scroll activity
- `Alt+Up` / `Alt+Down`: scroll activity by line
- `Ctrl+N`: new session
- `Ctrl+R`: resume latest session
- `Ctrl+P`: toggle `build` / `plan` mode for new sessions
- `Ctrl+A`: toggle approval policy between `allow-all` and `conservative`
- `Ctrl+L`: clear local panes
- `?`: help
- `Ctrl+C`: quit

## Slash Commands

- `/new`: start a fresh session
- `/resume`: resume the latest saved session
- `/sessions`: open the current directory session picker
- `/plan`: switch to the `plan` agent and start a fresh session
- `/build`: switch to the `build` agent and start a fresh session
- `/mode build|plan`: explicitly choose the agent mode and start a fresh session
- `/model`: open a model selector and switch the current session model in place
- `/approvals conservative|allow-all`: change approval policy
- `/clear`: clear local transcript/activity panes
- `/quit`: exit `sysiphus`
- `/help`: open help

## Notes

- The default approval flow is `allow-all` so normal tool use proceeds without extra friction. `conservative` is the opt-in guarded mode that still blocks obviously high-risk system commands.
- Permission events are shown in the UI so the next iteration can turn them into a true interactive approval modal.
- Because the Copilot SDK is still preview, `internal/copilot` keeps the SDK-specific integration isolated from the TUI state.
- Agent switching currently creates a new session because this MVP binds the selected custom agent at session creation time.
- `sysiphus` now keeps a local session index under `~/.local/share/sysiphus`, scoped to the exact launch directory, so `/sessions` can show prior sessions for the current workspace.
