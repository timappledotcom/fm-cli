# FM-Cli

A minimalist, terminal-based user interface (TUI) for Fastmail, built in Go.

## Features

- **Mailbox Navigation**: Browse your folders and see unread counts.
- **Email Reading**: View threads and read emails (supports Plain Text and HTML-to-Markdown rendering).
- **Composition**: Write emails using your preferred `$EDITOR` (Vim, Nano, etc.).
- **Clickable Links**: Supports OSC 8 hyperlinks for supported terminals.
- **Detailed Headers**: Toggle expanded email headers.
- **Secure Auth**: Stores your API token securely in the system keyring.

## Installation

### Prerequisites

- Go 1.25+
- A Fastmail account with an API Token.

### Build

```bash
git clone https://github.com/timappledotcom/fm-cli.git
cd fm-cli
go build ./cmd/fm-cli
```

## Usage

1. **Login**:
   ```bash
   ./fm-cli login
   ```
   Paste your Fastmail API token when prompted.

2. **Run**:
   ```bash
   ./fm-cli
   ```

### Controls

| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate up/down |
| `Enter` / `l` | Select mailbox / Open email |
| `h` / `Esc` | Go back |
| `c` | Compose new email |
| `m` | Toggle detailed headers (in email view) |
| `q` | Quit |
