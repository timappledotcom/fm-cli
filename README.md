# FM-Cli

A minimalist, terminal-based user interface (TUI) for Fastmail, built in Go.

## Features

- **Mailbox Navigation**: Browse your folders and see unread counts.
- **Email Reading**: View threads and read emails (supports Plain Text and HTML-to-Markdown rendering).
- **Composition**: Write emails using your preferred `$EDITOR` (Vim, Nano, etc.).
- **Multiple Identities**: Select from your configured Fastmail sending addresses.
- **Reply & Forward**: Reply to sender, reply all, or forward emails with quoted content.
- **Draft Management**: Save, edit, and send drafts.
- **Email Actions**: Mark read/unread, flag, archive, and delete emails.
- **Pagination**: Infinite scroll through large mailboxes.
- **Auto-Refresh**: Automatic sync with server after actions.
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

#### Mailbox List
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate up/down |
| `Enter` / `l` | Open mailbox |
| `r` | Refresh |
| `c` | Compose new email |
| `q` | Quit |

#### Email List
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate up/down |
| `Enter` / `l` | Open email |
| `h` / `Esc` | Go back to mailboxes |
| `u` | Toggle read/unread |
| `f` | Toggle flagged |
| `e` | Archive |
| `d` / `Backspace` | Delete |
| `r` | Refresh |
| `c` | Compose new email |

#### Email View
| Key | Action |
| --- | --- |
| `h` / `Esc` | Go back to email list |
| `R` | Reply to sender |
| `A` | Reply all |
| `F` | Forward |
| `m` | Toggle detailed headers |
| `e` | Edit (drafts only) |

#### Compose
| Key | Action |
| --- | --- |
| `Tab` | Cycle through sending identities |
| `Enter` | Continue to next field / Open editor |
| `Esc` | Cancel / Go back |

#### Send Confirmation
| Key | Action |
| --- | --- |
| `y` | Send email |
| `s` | Save as draft |
| `e` | Edit body |
| `n` | Cancel |
| `Tab` | Change sending identity |
