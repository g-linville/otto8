# Obot Setup

Obot Setup is the GUI setup application installed by the macOS package. It is
implemented with Fyne so the same orchestration code can be reused for Windows
later, while the current packaging target remains macOS.

The app shells out to the installed CLI instead of importing Obot setup logic.
For macOS, it invokes:

```bash
/usr/local/bin/obot setup status --json
/usr/local/bin/obot setup detect-agents --json
```

## Development

Run focused tests:

```bash
go test ./internal/...
```

Run the app on macOS:

```bash
go run ./cmd/obot-setup
```

Package a local macOS app bundle after installing Fyne's CLI:

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin -name "Obot Setup" -appID ai.obot.setup ./cmd/obot-setup
```
