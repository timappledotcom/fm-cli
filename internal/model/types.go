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
	ThreadID   string
	MailboxIDs []string
	Body       string
}
