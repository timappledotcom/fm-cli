# FM-Cli

A minimalist, terminal-based user interface (TUI) for Fastmail, built in Go.

## Features

- **Main Menu**: Quick access to Mail, Calendar, Contacts, and Settings.
- **Mailbox Navigation**: Browse your folders and see unread counts.
- **Email Reading**: View threads and read emails (supports Plain Text and HTML-to-Markdown rendering).
- **Composition**: Write emails using your preferred `$EDITOR` (Vim, Nano, etc.).
- **Multiple Identities**: Select from your configured Fastmail sending addresses.
- **Reply & Forward**: Reply to sender, reply all, or forward emails with quoted content.
- **Draft Management**: Save, edit, and send drafts.
- **Email Actions**: Mark read/unread, flag, archive, and delete emails.
- **Calendar**: View upcoming events in an agenda view, create and edit events (via CalDAV).
- **Contacts**: Browse your address book, add and edit contacts (via CardDAV).
- **Pagination**: Infinite scroll through large mailboxes.
- **Auto-Refresh**: Automatic sync with server after actions.
- **Offline Mode**: Store emails locally for offline access. Drafts created offline sync when back online.
- **Clickable Links**: Supports OSC 8 hyperlinks for supported terminals.
- **Detailed Headers**: Toggle expanded email headers.
- **Secure Auth**: Stores your credentials securely in the system keyring.

## Installation

### Prerequisites

- Go 1.25+
- A Fastmail account with:
  - An **API Token** (for email access via JMAP)
  - An **App Password** (optional, for calendar/contacts via CalDAV/CardDAV)
- CGO enabled (for SQLite support in offline mode)

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
   You will be prompted for:
   - Your Fastmail email address
   - Your API Token (required for email)
   - Your App Password (optional, enables calendar/contacts)

2. **Run**:
   ```bash
   ./fm-cli
   ```

### Getting Your Credentials

1. **API Token** (for email):
   - Go to Fastmail Settings → Privacy & Security → Integrations → API Tokens
   - Create a new token with Mail and Submission permissions

2. **App Password** (for calendar/contacts):
   - Go to Fastmail Settings → Privacy & Security → Integrations → App Passwords
   - Create a new password with "Mail, Contacts & Calendars" access
   - This uses CalDAV/CardDAV protocols which require a separate app password

### Commands

| Command | Description |
| --- | --- |
| `fm-cli` | Start the TUI |
| `fm-cli login` | Store API token in system keychain |
| `fm-cli logout` | Remove API token from keychain |
| `fm-cli settings` | View/modify settings |
| `fm-cli settings offline on` | Enable offline mode |
| `fm-cli settings offline off` | Disable offline mode |
| `fm-cli sync` | Sync pending offline changes |
| `fm-cli help` | Show help |

### Offline Mode

Enable offline mode to store emails locally:

```bash
./fm-cli settings offline on
```

When offline mode is enabled:
- Emails and mailboxes are cached locally in SQLite
- You can read cached emails without internet
- Drafts created offline are queued for sync
- Run `fm-cli sync` when back online to push changes

Data is stored in `~/.config/fm-cli/emails.db`.

### Controls

#### Global Navigation
| Key | Action |
| --- | --- |
| `0` | Return to main menu |
| `1` | Go to Mail |
| `2` | Go to Calendar |
| `3` | Go to Contacts |
| `4` | Go to Settings |
| `q` | Quit (from main menu) |

#### Main Menu
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate up/down |
| `Enter` / `l` | Select item |
| `m` | Go to Mail |
| `c` | Go to Calendar |
| `o` | Go to Contacts |
| `s` | Go to Settings |

#### Mailbox List
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate up/down |
| `Enter` / `l` | Open mailbox |
| `h` / `Esc` | Back to main menu |
| `r` | Refresh |
| `c` | Compose new email |

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

#### Calendar (Agenda View)
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate events |
| `Enter` / `l` | View event details |
| `n` | Create new event |
| `e` | Edit event (from details view) |
| `d` | Delete event |
| `r` | Refresh |
| `h` / `Esc` | Back (from details) or to menu |

#### Calendar Event Editor
| Key | Action |
| --- | --- |
| `Enter` | Save event |
| `Esc` | Cancel |

#### Contacts
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate contacts |
| `Enter` / `l` | View contact details |
| `n` | Create new contact |
| `e` | Edit contact (from details view) |
| `d` | Delete contact |
| `r` | Refresh |
| `h` / `Esc` | Back (from details) or to menu |

#### Contact Editor
| Key | Action |
| --- | --- |
| `Tab` | Move to next field |
| `Enter` | Save contact |
| `Esc` | Cancel |

#### Settings
| Key | Action |
| --- | --- |
| `j` / `k` (or Arrows) | Navigate |
| `Enter` | Toggle setting |
| `h` / `Esc` / `0` | Back to main menu |
