package model

// Mailbox represents a simplified JMAP mailbox for the TUI.
type Mailbox struct {
	ID          string
	Name        string
	UnreadCount int
	Role        string // e.g., "inbox", "archive", "snoozed"
	ParentID    string
	SortOrder   int
}

// AppConfig holds configuration details like the API token.
type AppConfig struct {
	APIToken string
	JMAPCore string // URL for the JMAP session resource
}

// Settings holds user preferences
type Settings struct {
	OfflineMode    bool // Store emails locally for offline access
	SyncOnStartup  bool // Sync with server on startup
	AutoSync       bool // Auto-sync after actions
}

// DefaultSettings returns the default settings
func DefaultSettings() Settings {
	return Settings{
		OfflineMode:   false,
		SyncOnStartup: true,
		AutoSync:      true,
	}
}

// Email represents a simplified email message for the TUI.
type Email struct {
	ID         string
	Subject    string
	From       string
	To         string
	Cc         string
	Bcc        string
	ReplyTo    string
	Preview    string
	Date       string
	IsUnread   bool
	IsFlagged  bool
	IsDraft    bool
	IsLocal    bool // True if this is a local-only draft not yet synced
	ThreadID   string
	MailboxIDs []string
	Body       string
}
