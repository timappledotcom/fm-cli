package tui

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"fm-cli/internal/api"
	"fm-cli/internal/model"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionState indicates the current view
type sessionState int

const (
	viewMailboxes sessionState = iota
	viewEmails
	viewBody
	viewComposeTo
	viewComposeSubject
	viewComposeConfirm
)

// Styles
var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	// Mailbox Styles
	mailboxStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			PaddingLeft(1)

	selectedMailboxStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("57")).
				PaddingLeft(1)

	// Email Styles
	emailItemStyle = lipgloss.NewStyle().
			PaddingLeft(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color("240"))

	selectedEmailItemStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(lipgloss.Color("229")).
				Background(lipgloss.Color("57")).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(lipgloss.Color("57"))

	unreadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)
)

// msg types
type mailboxesLoadedMsg []model.Mailbox
type emailsLoadedMsg []model.Email
type emailsRefreshedMsg []model.Email // For refresh without appending
type emailBodyLoadedMsg string
type editorFinishedMsg struct{ err error }
type emailSentMsg struct{}
type draftSavedMsg struct{}
type emailDeletedMsg struct{}
type identitiesLoadedMsg []string
type errorMsg error

// Model implementation
type Model struct {
	client *api.Client
	state  sessionState

	// Mailbox View Data
	mailboxes []model.Mailbox
	mbCursor  int

	// Email View Data
	emails      []model.Email
	emailCursor int
	emailOffset int
	loading     bool
	canLoadMore bool // If true, hitting bottom loads more

	// Body View Data
	bodyContent string
	showDetails bool // Toggle expanded headers

	// Composition Data
	inputTo      textinput.Model
	inputSubject textinput.Model
	composeBody  string
	tempFile     string
	draftID      string   // If editing a draft
	identities   []string // Available sending identities (email addresses)
	identityIdx  int      // Currently selected identity index

	err    error
	width  int
	height int
}

func NewModel(client *api.Client) Model {
	tiTo := textinput.New()
	tiTo.Placeholder = "recipient@example.com"
	tiTo.Focus()

	tiSubj := textinput.New()
	tiSubj.Placeholder = "Subject"

	return Model{
		client:       client,
		state:        viewMailboxes,
		inputTo:      tiTo,
		inputSubject: tiSubj,
		loading:      true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchMailboxesCmd(m.client), fetchIdentitiesCmd(m.client))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Global / Async Message Handling (Higher Priority)
	switch msg := msg.(type) {
	case editorFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		content, err := ioutil.ReadFile(m.tempFile)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.composeBody = string(content)
		m.state = viewComposeConfirm
		return m, nil

	case mailboxesLoadedMsg:
		m.mailboxes = msg
		m.loading = false
		return m, nil

	case emailsLoadedMsg:
		newEmails := []model.Email(msg)
		if len(newEmails) < 20 {
			m.canLoadMore = false
		} else {
			m.canLoadMore = true
		}
		m.emails = append(m.emails, newEmails...)
		m.loading = false
		return m, nil

	case emailsRefreshedMsg:
		// Replace emails instead of appending (for refresh)
		m.emails = []model.Email(msg)
		m.emailOffset = 0
		m.emailCursor = 0
		if len(m.emails) < 20 {
			m.canLoadMore = false
		} else {
			m.canLoadMore = true
		}
		m.loading = false
		return m, nil

	case emailBodyLoadedMsg:
		m.bodyContent = string(msg)
		m.loading = false
		return m, nil

	case identitiesLoadedMsg:
		m.identities = msg
		return m, nil

	case draftSavedMsg:
		m.loading = false
		m.state = viewMailboxes
		os.Remove(m.tempFile)
		return m, fetchMailboxesCmd(m.client)

	case emailSentMsg:
		m.loading = false
		m.state = viewMailboxes
		os.Remove(m.tempFile)
		return m, fetchMailboxesCmd(m.client)

	case emailDeletedMsg:
		m.loading = false
		// Refresh mailbox counts after delete
		return m, fetchMailboxesCmd(m.client)

	case errorMsg:
		m.err = msg
		m.loading = false
		return m, nil
	
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Don't return, let UI resize if needed (though mostly static)
	}

	// Handle Composition States
	if m.state == viewComposeTo {
		m.inputTo, cmd = m.inputTo.Update(msg)
		
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				m.state = viewComposeSubject
				m.inputTo.Blur()
				m.inputSubject.Focus()
				return m, textinput.Blink
			case tea.KeyTab:
				if len(m.identities) > 1 {
					m.identityIdx = (m.identityIdx + 1) % len(m.identities)
				}
				return m, nil
			case tea.KeyEsc:
				m.state = viewMailboxes
				m.inputTo.Blur()
				return m, nil
			// Global Quit check (optional here or fallthrough? better usually global first)
			case tea.KeyCtrlC:
				return m, tea.Quit
			}
		}
		return m, cmd
	}

	if m.state == viewComposeSubject {
		m.inputSubject, cmd = m.inputSubject.Update(msg)
		
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.Type {
			case tea.KeyEnter:
				// Create Temp File
				f, err := ioutil.TempFile("", "fm-cli-*.txt")
				if err != nil {
					m.err = err
					return m, nil
				}
				
				// Write existing body content to file if available
				if m.composeBody != "" {
					if _, err := f.WriteString(m.composeBody); err != nil {
						f.Close()
						m.err = err
						return m, nil
					}
				}
				
				m.tempFile = f.Name()
				f.Close()

				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "nano"
				}
				c := exec.Command(editor, m.tempFile)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return editorFinishedMsg{err}
				})
			case tea.KeyTab:
				if len(m.identities) > 1 {
					m.identityIdx = (m.identityIdx + 1) % len(m.identities)
				}
				return m, nil
			case tea.KeyEsc:
				m.state = viewComposeTo
				m.inputSubject.Blur()
				m.inputTo.Focus()
				return m, textinput.Blink
			case tea.KeyCtrlC:
				return m, tea.Quit
			}
		}
		return m, cmd
	}

	if m.state == viewComposeConfirm {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "y", "Y":
				m.loading = true
				fromAddr := ""
				if len(m.identities) > 0 {
					fromAddr = m.identities[m.identityIdx]
				}
				return m, sendEmailCmd(m.client, m.draftID, fromAddr, m.inputTo.Value(), m.inputSubject.Value(), m.composeBody)
			case "s", "S":
				m.loading = true
				fromAddr := ""
				if len(m.identities) > 0 {
					fromAddr = m.identities[m.identityIdx]
				}
				return m, saveDraftCmd(m.client, m.draftID, fromAddr, m.inputTo.Value(), m.inputSubject.Value(), m.composeBody)
			case "n", "N":
				m.state = viewMailboxes
				m.composeBody = ""
				os.Remove(m.tempFile)
				return m, nil
			case "e", "E":
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "nano"
				}
				c := exec.Command(editor, m.tempFile)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return editorFinishedMsg{err}
				})
			case "tab":
				if len(m.identities) > 1 {
					m.identityIdx = (m.identityIdx + 1) % len(m.identities)
				}
				return m, nil
			case "ctrl+c":
				return m, tea.Quit
			}
		}
		return m, nil
	}

	// Normal Navigation States
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "d", "backspace":
			if m.state == viewEmails && len(m.emails) > 0 {
				m.loading = true
				selectedEmail := m.emails[m.emailCursor]
				// Optimistic UI update
				if m.emailCursor < len(m.emails)-1 {
					m.emails = append(m.emails[:m.emailCursor], m.emails[m.emailCursor+1:]...)
				} else {
					m.emails = m.emails[:m.emailCursor]
					if m.emailCursor > 0 {
						m.emailCursor--
					}
				}
				return m, deleteEmailCmd(m.client, selectedEmail.ID)
			}

		case "u":
			if m.state == viewEmails && len(m.emails) > 0 {
				selectedEmail := m.emails[m.emailCursor]
				newState := !selectedEmail.IsUnread
				m.emails[m.emailCursor].IsUnread = newState
				return m, toggleUnreadCmd(m.client, selectedEmail.ID, newState)
			}

		case "f":
			if m.state == viewEmails && len(m.emails) > 0 {
				selectedEmail := m.emails[m.emailCursor]
				newState := !selectedEmail.IsFlagged
				m.emails[m.emailCursor].IsFlagged = newState
				return m, toggleFlaggedCmd(m.client, selectedEmail.ID, newState)
			}
		
		case "e":
			if m.state == viewEmails && len(m.emails) > 0 {
				targetMBID := ""
				// Find Archive Mailbox ID
				for _, mb := range m.mailboxes {
					if mb.Role == "archive" {
						targetMBID = mb.ID
						break
					}
				}

				if targetMBID != "" {
					m.loading = true
					selectedEmail := m.emails[m.emailCursor]
					currentMBID := m.mailboxes[m.mbCursor].ID
					
					// Optimistic UI update
					if m.emailCursor < len(m.emails)-1 {
						m.emails = append(m.emails[:m.emailCursor], m.emails[m.emailCursor+1:]...)
					} else {
						m.emails = m.emails[:m.emailCursor]
						if m.emailCursor > 0 {
							m.emailCursor--
						}
					}
					
					return m, moveEmailCmd(m.client, selectedEmail.ID, currentMBID, targetMBID)
				}
			} else if m.state == viewBody {
				// If viewing a draft, 'e' edits it
				if len(m.emails) > m.emailCursor {
					selectedEmail := m.emails[m.emailCursor]
					if selectedEmail.IsDraft {
						m.state = viewComposeTo
						m.draftID = selectedEmail.ID
						m.inputTo.SetValue(selectedEmail.To)
						m.inputSubject.SetValue(selectedEmail.Subject)
						
						// Prepare body
						body := m.bodyContent
						if strings.HasPrefix(body, "[Converted HTML]\n") {
							body = strings.TrimPrefix(body, "[Converted HTML]\n")
						}
						m.composeBody = body
						
						// Determine focus
						if m.inputTo.Value() == "" {
							m.state = viewComposeTo
							m.inputTo.Focus()
						} else {
							m.state = viewComposeSubject
							m.inputTo.Blur()
							m.inputSubject.Focus()
						}
						return m, textinput.Blink
					}
				}
			}

		case "c":
			m.state = viewComposeTo
			m.draftID = "" // New email
			m.inputTo.SetValue("")
			m.inputSubject.SetValue("")
			m.composeBody = ""
			m.inputTo.Focus()
			return m, textinput.Blink

		case "R": // Reply to sender
			if m.state == viewBody && len(m.emails) > 0 {
				selectedEmail := m.emails[m.emailCursor]
				m.state = viewComposeTo
				m.draftID = ""
				// Use ReplyTo if available, otherwise From
				replyTo := selectedEmail.From
				if selectedEmail.ReplyTo != "" {
					replyTo = selectedEmail.ReplyTo
				}
				m.inputTo.SetValue(replyTo)
				// Add Re: prefix if not already present
				subject := selectedEmail.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				m.inputSubject.SetValue(subject)
				// Quote original message
				m.composeBody = fmt.Sprintf("\n\n--- Original Message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n%s",
					selectedEmail.From, selectedEmail.Date, selectedEmail.Subject, m.bodyContent)
				m.inputTo.Focus()
				return m, textinput.Blink
			}

		case "A": // Reply all
			if m.state == viewBody && len(m.emails) > 0 {
				selectedEmail := m.emails[m.emailCursor]
				m.state = viewComposeTo
				m.draftID = ""
				// Combine From (or ReplyTo), To, and Cc for reply-all
				var recipients []string
				replyTo := selectedEmail.From
				if selectedEmail.ReplyTo != "" {
					replyTo = selectedEmail.ReplyTo
				}
				recipients = append(recipients, replyTo)
				if selectedEmail.To != "" {
					recipients = append(recipients, selectedEmail.To)
				}
				if selectedEmail.Cc != "" {
					recipients = append(recipients, selectedEmail.Cc)
				}
				m.inputTo.SetValue(strings.Join(recipients, ", "))
				// Add Re: prefix if not already present
				subject := selectedEmail.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}
				m.inputSubject.SetValue(subject)
				// Quote original message
				m.composeBody = fmt.Sprintf("\n\n--- Original Message ---\nFrom: %s\nDate: %s\nSubject: %s\n\n%s",
					selectedEmail.From, selectedEmail.Date, selectedEmail.Subject, m.bodyContent)
				m.inputTo.Focus()
				return m, textinput.Blink
			}

		case "F": // Forward
			if m.state == viewBody && len(m.emails) > 0 {
				selectedEmail := m.emails[m.emailCursor]
				m.state = viewComposeTo
				m.draftID = ""
				m.inputTo.SetValue("") // User needs to enter recipient
				// Add Fwd: prefix if not already present
				subject := selectedEmail.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "fwd:") && !strings.HasPrefix(strings.ToLower(subject), "fw:") {
					subject = "Fwd: " + subject
				}
				m.inputSubject.SetValue(subject)
				// Include forwarded message
				m.composeBody = fmt.Sprintf("\n\n--- Forwarded Message ---\nFrom: %s\nTo: %s\nDate: %s\nSubject: %s\n\n%s",
					selectedEmail.From, selectedEmail.To, selectedEmail.Date, selectedEmail.Subject, m.bodyContent)
				m.inputTo.Focus()
				return m, textinput.Blink
			}

		case "m":
			if m.state == viewBody {
				m.showDetails = !m.showDetails
				return m, nil
			}

		case "up", "k":
			if m.state == viewMailboxes {
				if m.mbCursor > 0 {
					m.mbCursor--
				}
			} else if m.state == viewEmails {
				if m.emailCursor > 0 {
					m.emailCursor--
					if m.emailCursor < m.emailOffset {
						m.emailOffset = m.emailCursor
					}
				}
			}

		case "down", "j":
			if m.state == viewMailboxes {
				if m.mbCursor < len(m.mailboxes)-1 {
					m.mbCursor++
				}
			} else if m.state == viewEmails {
				// Dynamic page height
				headerHeight := 5
				footerHeight := 2
				pageHeight := m.height - headerHeight - footerHeight
				if pageHeight < 5 {
					pageHeight = 5
				}

				if m.emailCursor < len(m.emails)-1 {
					m.emailCursor++
					if m.emailCursor >= m.emailOffset+pageHeight {
						m.emailOffset++
					}
				} else if m.canLoadMore && !m.loading {
					m.loading = true
					selectedMB := m.mailboxes[m.mbCursor]
					return m, fetchEmailsCmd(m.client, selectedMB.ID, len(m.emails))
				}
			}

		case "enter", "right", "l":
			if m.state == viewMailboxes && len(m.mailboxes) > 0 {
				m.state = viewEmails
				m.emailCursor = 0 // reset cursor
				m.emailOffset = 0 // reset offset
				m.emails = nil    // clear previous
				m.loading = true
				m.canLoadMore = true
				selectedMB := m.mailboxes[m.mbCursor]
				return m, fetchEmailsCmd(m.client, selectedMB.ID, 0)
			} else if m.state == viewEmails && len(m.emails) > 0 {
				// Always go to preview first, even for drafts
				m.state = viewBody
				m.loading = true
				selectedEmail := m.emails[m.emailCursor]
				return m, fetchEmailBodyCmd(m.client, selectedEmail.ID)
			}

		case "esc", "left", "h":
			if m.state == viewEmails {
				m.state = viewMailboxes
				m.emails = nil
				// Refresh mailbox counts when returning
				return m, fetchMailboxesCmd(m.client)
			} else if m.state == viewBody {
				m.state = viewEmails
				m.bodyContent = ""
			}

		case "r":
			// Manual refresh
			if m.state == viewMailboxes {
				m.loading = true
				return m, fetchMailboxesCmd(m.client)
			} else if m.state == viewEmails && len(m.mailboxes) > 0 {
				m.loading = true
				selectedMB := m.mailboxes[m.mbCursor]
				return m, tea.Batch(fetchMailboxesCmd(m.client), refreshEmailsCmd(m.client, selectedMB.ID))
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case mailboxesLoadedMsg:
		m.mailboxes = msg
		m.loading = false

	case emailsLoadedMsg:
		newEmails := []model.Email(msg)
		if len(newEmails) < 20 {
			m.canLoadMore = false
		} else {
			m.canLoadMore = true
		}
		m.emails = append(m.emails, newEmails...)
		m.loading = false

	case emailBodyLoadedMsg:
		m.bodyContent = string(msg)
		m.loading = false
		
		// If we are loading a draft to edit:
		if m.draftID != "" && (m.state == viewComposeTo || m.state == viewEmails) {
			// We came here from selecting a draft
			// Clean up "To" field (remove Name <Email> format to just Email if possible, or leave it)
			// JMAP usually handles Name <Email> in To field ok on sending? 
			// Actually our SendEmail uses Email struct which parses it or expects raw.
			// Ideally we should parse it. For now, leave as is.
			
			// Clean body: Remove [Converted HTML] header if present?
			// Since we want to edit the raw text.
			// The fetchEmailBody returns converted text.
			// Ideally we want raw textBody from API.
			// Current API `FetchEmailBody` tries text then html->text.
			body := string(msg)
			if strings.HasPrefix(body, "[Converted HTML]\n") {
				body = strings.TrimPrefix(body, "[Converted HTML]\n")
			}
			m.composeBody = body
			
			// Determine where to focus
			if m.inputTo.Value() == "" {
				m.state = viewComposeTo
				m.inputTo.Focus()
			} else {
				m.state = viewComposeSubject
				m.inputTo.Blur()
				m.inputSubject.Focus()
			}
		}

	case editorFinishedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		content, err := ioutil.ReadFile(m.tempFile)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.composeBody = string(content)
		m.state = viewComposeConfirm
		return m, nil

	case emailDeletedMsg:
		m.loading = false
		return m, nil

	case errorMsg:
		m.err = msg
		m.loading = false
	}

	return m, nil
}

// Helper to make links clickable (OSC 8)
func linkify(text string) string {
	// 1. Convert Markdown links: [Title](URL) -> OSC 8 link
	reMD := regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)]+)\)`)
	text = reMD.ReplaceAllString(text, "\x1b]8;;$2\x1b\\$1\x1b]8;;\x1b\\")

	// 2. Convert Bare URLs: https://google.com -> OSC 8 link
	// We use a negative lookbehind/lookahead logic implicitly by replacing existing OSC 8 sequences?
	// Or simpler: Just find http... that is NOT preceded by ]8;;
	// But regex in Go doesn't support lookbehind.
	// So we can assume step 1 handles Markdown links. Now we have bare URLs in the remaining text.
	// Note: Step 1 results contain `https://` inside the escape sequence.
	// So if we run a naive replace "http...", we might double-link inside the escape sequence.
	// A robust way works on non-linked text chunks, but that's complex.
	// Simple Hack: Since Markdown conversion usually consumes the URL, let's just do bare URL replacement for plain text mostly.
	// Or, use a regex that ignores things inside escape codes? Hard.

	// Alternative: only match URLs not preceded by `;;` (part of OSC 8 opening) or `(` (part of MD structure we missed?).
	// The safest bet for this simple TUI:
	// If text starts with "[Converted HTML]" it likely has MD links.
	// If it doesn't, it's Plain Text and has bare links.

	if strings.Contains(text, "[Converted HTML]") {
		// It's mostly Markdown links now.
		// There might be bare links in MD too, but let's trust MD converter.
		return text
	}

	// Plain text mode: Wrap all bare URLs
	reURL := regexp.MustCompile(`(https?://[^\s()<>"]+)`)
	return reURL.ReplaceAllString(text, "\x1b]8;;$1\x1b\\$1\x1b]8;;\x1b\\")
}

func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}

	s := strings.Builder{}
	s.WriteString(titleStyle.Render("FM-CLI"))
	s.WriteString(" ")

	// Breadcrumbs
	if (m.state == viewEmails || m.state == viewBody) && len(m.mailboxes) > 0 {
		mb := m.mailboxes[m.mbCursor]
		s.WriteString(fmt.Sprintf("> %s", mb.Name))
	}
	s.WriteString("\n\n")

	if m.state == viewMailboxes {
		if m.loading {
			s.WriteString("Loading mailboxes...")
		} else if len(m.mailboxes) == 0 {
			s.WriteString("No mailboxes found.")
		}
		for i, mb := range m.mailboxes {
			cursor := " "
			style := mailboxStyle

			if i == m.mbCursor {
				cursor = ">"
				style = selectedMailboxStyle
			}

			label := fmt.Sprintf("%s %s (%d)", cursor, mb.Name, mb.UnreadCount)
			s.WriteString(style.Render(label) + "\n")
		}
		s.WriteString("\n(j/k navigate, enter/l open, r: refresh, c: compose)")

	} else if m.state == viewEmails {
		if m.loading {
			s.WriteString("Loading emails using JMAP...\n")
		} else if len(m.emails) == 0 {
			s.WriteString("No emails found.")
		} else {
			// Basic Render Loop for Emails
			headerHeight := 5
			footerHeight := 2
			pageHeight := m.height - headerHeight - footerHeight
			if pageHeight < 5 {
				pageHeight = 5
			}

			start := m.emailOffset
			end := start + pageHeight
			if end > len(m.emails) {
				end = len(m.emails)
			}

			for i := start; i < end; i++ {
				e := m.emails[i]
				style := emailItemStyle
				if i == m.emailCursor {
					style = selectedEmailItemStyle
				}

				unreadMarker := " "
				if e.IsUnread {
					unreadMarker = "*"
				}
				
				flagMarker := " "
				if e.IsFlagged {
					flagMarker = "!"
				}

				// Format: * ! [Date] From: Subject
				line := fmt.Sprintf("%s%s [%s] %-20s %s", unreadMarker, flagMarker, e.Date, e.From, e.Subject)

				if e.IsUnread {
					line = unreadStyle.Render(line)
				}

				s.WriteString(style.Render(line) + "\n")
			}
		}
		s.WriteString("\n(h/esc back, j/k navigate, r: refresh, u: read/unread, f: flag, e: archive, d: delete, c: compose)")
	
	} else if m.state == viewBody {
		if m.loading {
			s.WriteString("Loading content...\n")
		} else {
			if len(m.emails) > m.emailCursor {
				e := m.emails[m.emailCursor]
				s.WriteString(fmt.Sprintf("Subject: %s\nFrom:    %s\nDate:    %s\n", e.Subject, e.From, e.Date))

				if m.showDetails {
					if e.To != "" {
						s.WriteString(fmt.Sprintf("To:      %s\n", e.To))
					}
					if e.Cc != "" {
						s.WriteString(fmt.Sprintf("Cc:      %s\n", e.Cc))
					}
					if e.Bcc != "" {
						s.WriteString(fmt.Sprintf("Bcc:     %s\n", e.Bcc))
					}
					if e.ReplyTo != "" {
						s.WriteString(fmt.Sprintf("ReplyTo: %s\n", e.ReplyTo))
					}
					s.WriteString(fmt.Sprintf("ID:      %s\n", e.ID))
					s.WriteString(fmt.Sprintf("Mailboxes: %v\n", e.MailboxIDs))
				}

				s.WriteString("--------------------------------------------------\n\n")
				
				// Render body with clickable links
				content := linkify(m.bodyContent)
				s.WriteString(content)
			}
		}
		
		help := "\n\n(h/esc: back, R: reply, A: reply all, F: forward, m: toggle details)"
		if len(m.emails) > m.emailCursor && m.emails[m.emailCursor].IsDraft {
			help = "\n\n(h/esc: back, e: edit draft, m: toggle details)"
		}
		s.WriteString(help)

	} else if m.state == viewComposeTo {
		s.WriteString("Compose New Email\n\n")
		fromAddr := "(loading...)"
		if len(m.identities) > 0 {
			fromAddr = m.identities[m.identityIdx]
		}
		s.WriteString("From: " + fromAddr + "  [Tab to change]\n")
		s.WriteString("To: " + m.inputTo.View() + "\n")
		s.WriteString("\n(Enter to continue, Tab to cycle From, Esc to cancel)")

	} else if m.state == viewComposeSubject {
		s.WriteString("Compose New Email\n\n")
		fromAddr := ""
		if len(m.identities) > 0 {
			fromAddr = m.identities[m.identityIdx]
		}
		s.WriteString("From: " + fromAddr + "\n")
		s.WriteString("To: " + m.inputTo.Value() + "\n")
		s.WriteString("Subject: " + m.inputSubject.View() + "\n")
		s.WriteString("\n(Enter to write body in $EDITOR, Tab to cycle From, Esc to back)")

	} else if m.state == viewComposeConfirm {
		s.WriteString("Confirm Send?\n\n")
		fromAddr := ""
		if len(m.identities) > 0 {
			fromAddr = m.identities[m.identityIdx]
		}
		s.WriteString("From: " + fromAddr + "\n")
		s.WriteString("To: " + m.inputTo.Value() + "\n")
		s.WriteString("Subject: " + m.inputSubject.Value() + "\n")
		s.WriteString("Body Preview:\n")
		
		preview := m.composeBody
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		s.WriteString(preview + "\n")
		
		if m.loading {
			s.WriteString("\nSENDING...\n")
		} else {
			s.WriteString("\n(y) Send  (s) Save Draft  (n) Cancel  (e) Edit Body  (Tab) Change From")
		}
	}

	return appStyle.Render(s.String())
}

// Commands
func saveDraftCmd(client *api.Client, draftID, from, to, subject, body string) tea.Cmd {
	return func() tea.Msg {
		err := client.SaveDraft(draftID, from, to, subject, body)
		if err != nil {
			return errorMsg(err)
		}
		return draftSavedMsg{}
	}
}

func sendEmailCmd(client *api.Client, draftID, from, to, subject, body string) tea.Cmd {
	return func() tea.Msg {
		err := client.SendEmail(draftID, from, to, subject, body)
		if err != nil {
			return errorMsg(err)
		}
		return emailSentMsg{}
	}
}

func moveEmailCmd(client *api.Client, emailID, fromMBID, toMBID string) tea.Cmd {
	return func() tea.Msg {
		err := client.MoveEmail(emailID, fromMBID, toMBID)
		if err != nil {
			return errorMsg(err)
		}
		return emailDeletedMsg{} // Reuse deleted msg to clear loading state
	}
}

func deleteEmailCmd(client *api.Client, emailID string) tea.Cmd {
	return func() tea.Msg {
		err := client.DeleteEmail(emailID)
		if err != nil {
			return errorMsg(err)
		}
		return emailDeletedMsg{}
	}
}

func toggleUnreadCmd(client *api.Client, emailID string, isUnread bool) tea.Cmd {
	return func() tea.Msg {
		err := client.SetUnread(emailID, isUnread)
		if err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func toggleFlaggedCmd(client *api.Client, emailID string, isFlagged bool) tea.Cmd {
	return func() tea.Msg {
		err := client.SetFlagged(emailID, isFlagged)
		if err != nil {
			return errorMsg(err)
		}
		return nil
	}
}

func fetchMailboxesCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		mbs, err := client.FetchMailboxes()
		if err != nil {
			return errorMsg(err)
		}
		return mailboxesLoadedMsg(mbs)
	}
}

func fetchIdentitiesCmd(client *api.Client) tea.Cmd {
	return func() tea.Msg {
		identities, err := client.GetIdentities()
		if err != nil {
			return errorMsg(err)
		}
		var emails []string
		for _, id := range identities {
			emails = append(emails, id.Email)
		}
		return identitiesLoadedMsg(emails)
	}
}

func fetchEmailsCmd(client *api.Client, mailboxID string, offset int) tea.Cmd {
	return func() tea.Msg {
		emails, err := client.FetchEmails(mailboxID, offset)
		if err != nil {
			return errorMsg(err)
		}
		return emailsLoadedMsg(emails)
	}
}

func refreshEmailsCmd(client *api.Client, mailboxID string) tea.Cmd {
	return func() tea.Msg {
		emails, err := client.FetchEmails(mailboxID, 0)
		if err != nil {
			return errorMsg(err)
		}
		return emailsRefreshedMsg(emails)
	}
}

func fetchEmailBodyCmd(client *api.Client, emailID string) tea.Cmd {
return func() tea.Msg {
body, err := client.FetchEmailBody(emailID)
if err != nil {
return errorMsg(err)
}
return emailBodyLoadedMsg(body)
}
}
