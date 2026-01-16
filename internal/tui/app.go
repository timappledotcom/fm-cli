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
type emailBodyLoadedMsg string
type editorFinishedMsg struct{ err error }
type emailSentMsg struct{}
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
	return fetchMailboxesCmd(m.client)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

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
				return m, sendEmailCmd(m.client, m.inputTo.Value(), m.inputSubject.Value(), m.composeBody)
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

		case "c":
			m.state = viewComposeTo
			m.inputTo.SetValue("")
			m.inputSubject.SetValue("")
			m.inputTo.Focus()
			return m, textinput.Blink

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
				}
			}

		case "down", "j":
			if m.state == viewMailboxes {
				if m.mbCursor < len(m.mailboxes)-1 {
					m.mbCursor++
				}
			} else if m.state == viewEmails {
				if m.emailCursor < len(m.emails)-1 {
					m.emailCursor++
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
				m.emails = nil    // clear previous
				m.loading = true
				m.canLoadMore = true
				selectedMB := m.mailboxes[m.mbCursor]
				return m, fetchEmailsCmd(m.client, selectedMB.ID, 0)
			} else if m.state == viewEmails && len(m.emails) > 0 {
				m.state = viewBody
				m.loading = true
				selectedEmail := m.emails[m.emailCursor]
				return m, fetchEmailBodyCmd(m.client, selectedEmail.ID)
			}

		case "esc", "left", "h":
			if m.state == viewEmails {
				m.state = viewMailboxes
				m.emails = nil
			} else if m.state == viewBody {
				m.state = viewEmails
				m.bodyContent = ""
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
		// Hack to force repaint or ensure we are not in editor
		return m, tea.EnterAltScreen

	case emailSentMsg:
		m.loading = false
		m.state = viewMailboxes
		os.Remove(m.tempFile)
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
		s.WriteString("\n(j/k navigate, enter/l open)")

	} else if m.state == viewEmails {
		if m.loading {
			s.WriteString("Loading emails using JMAP...\n")
		} else if len(m.emails) == 0 {
			s.WriteString("No emails found.")
		} else {
			// Basic Render Loop for Emails
			// In a real app we would use bubbles/list with pagination
			limit := 20
			if limit > len(m.emails) {
				limit = len(m.emails)
			}

			for i := 0; i < limit; i++ {
				e := m.emails[i]
				style := emailItemStyle
				if i == m.emailCursor {
					style = selectedEmailItemStyle
				}

				unreadMarker := " "
				if e.IsUnread {
					unreadMarker = "*"
				}

				// Format: * [Date] From: Subject
				line := fmt.Sprintf("%s [%s] %-20s %s", unreadMarker, e.Date, e.From, e.Subject)

				if e.IsUnread {
					line = unreadStyle.Render(line)
				}

				s.WriteString(style.Render(line) + "\n")
			}
		}
		s.WriteString("\n(h/esc back, j/k navigate)")
	
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
		s.WriteString("\n\n(h/esc: back, m: toggle details)")

	} else if m.state == viewComposeTo {
		s.WriteString("Compose New Email\n\n")
		s.WriteString("To: " + m.inputTo.View() + "\n")
		s.WriteString("\n(Enter to continue, Esc to cancel)")

	} else if m.state == viewComposeSubject {
		s.WriteString("Compose New Email\n\n")
		s.WriteString("To: " + m.inputTo.Value() + "\n")
		s.WriteString("Subject: " + m.inputSubject.View() + "\n")
		s.WriteString("\n(Enter to write body in $EDITOR, Esc to back)")

	} else if m.state == viewComposeConfirm {
		s.WriteString("Confirm Send?\n\n")
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
			s.WriteString("\n(y) Send  (n) Cancel  (e) Edit Body")
		}
	}

	return appStyle.Render(s.String())
}

// Commands
func sendEmailCmd(client *api.Client, to, subject, body string) tea.Cmd {
	return func() tea.Msg {
		// Hardcoded "From" for now? Or fetch identity?
		// Client doesn't store "From". 
		// We'll use the Username used in login? 
		// We don't have it easily available here...
		// For now, let's assume the user enters it, OR we fetch identity.
		// WAIT: The user didn't enter "From".
		// We can get the Primary Identity from JMAP "Identity/get".
		// But I haven't implemented that.
		// Hack: use the "To" address as "From" if testing, or just try to find a valid From.
		// Actually, JMAP server often infers From if missing or matches auth.
		// Or I can use 'client.GetDefaultIdentity()'.
		// I'll try to find a default identity.
		// For now, let's just use a placeholder or ask Client to handle it.
		// I'll modify SendEmail to create a "From" if I can, or Client has `Session`.
		// But Client struct in `internal/api/jmap.go` exposes `Client *jmap.Client`.
		// `jmap.Client` has `Session` which has `PrimaryAccounts`.
		// But finding the email address... `Session.Accounts`...
		
		// Let's fallback to a simpler approach: 
		// Use a hardcoded "me" or similar, relying on Server default?
		// No, `SendEmail` in API requires `from`.
		// I will update `SendEmail` to fetch identity if `from` is empty.
		
		// For the TUI, I will pass empty "from" and let API handle it.
		err := client.SendEmail("", to, subject, body)
		if err != nil {
			return errorMsg(err)
		}
		return emailSentMsg{}
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

func fetchEmailsCmd(client *api.Client, mailboxID string, offset int) tea.Cmd {
	return func() tea.Msg {
		emails, err := client.FetchEmails(mailboxID, offset)
		if err != nil {
			return errorMsg(err)
		}
		return emailsLoadedMsg(emails)
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
