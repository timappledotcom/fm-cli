package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"fm-cli/internal/model"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"git.sr.ht/~rockorager/go-jmap/mail/emailsubmission"
	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"
)

type Client struct {
	Client  *jmap.Client
	Session *jmap.Session
}

const FastmailSessionURL = "https://api.fastmail.com/.well-known/jmap"

// NewClient initializes a JMAP client with the given token.
func NewClient(token string) (*Client, error) {
	// Initialize the JMAP client
	c := &jmap.Client{
		SessionEndpoint: FastmailSessionURL,
		HttpClient:      &http.Client{},
	}
	c.WithAccessToken(token)

	// Phase 1: Authentication & Session Discovery
	// We fetch the session object to discover capabilities and URLs.
	if err := c.Authenticate(); err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	return &Client{
		Client:  c,
		Session: c.Session,
	}, nil
}

func (c *Client) getMailAccountID() jmap.ID {
	if c.Session == nil {
		return ""
	}
	return c.Session.PrimaryAccounts[mail.URI]
}

// FetchMailboxes retrieves all mailboxes using standard JMAP calls.
func (c *Client) FetchMailboxes() ([]model.Mailbox, error) {
	var mailboxes []model.Mailbox

	// Create a Request
	req := &jmap.Request{}
	mGet := &mailbox.Get{
		Account: c.getMailAccountID(),
	}
	req.Invoke(mGet)

	// Execute the request
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("JMAP request failed: %w", err)
	}

	// Iterate over the responses
	for _, invocation := range resp.Responses {
		if errArgs, ok := invocation.Args.(*jmap.MethodError); ok {
			return nil, fmt.Errorf("JMAP method error: %s (type: %s)", invocation.Name, errArgs.Type)
		}
		if res, ok := invocation.Args.(*mailbox.GetResponse); ok {
			for _, m := range res.List {
				mailboxes = append(mailboxes, model.Mailbox{
					ID:          string(m.ID),
					Name:        m.Name,
					UnreadCount: int(m.UnreadThreads),
					Role:        string(m.Role),
					ParentID:    string(m.ParentID),
					SortOrder:   int(m.SortOrder),
				})
			}
		}
	}

	// Sort mailboxes by sort order or name for safety
	sort.Slice(mailboxes, func(i, j int) bool {
		return mailboxes[i].SortOrder < mailboxes[j].SortOrder
	})

	return mailboxes, nil
}

// FetchEmails retrieves emails for a specific mailbox.
func (c *Client) FetchEmails(mailboxID string, position int) ([]model.Email, error) {
	var emails []model.Email
	const limit = 20

	// 1. Email/query
	// Sequential fallback is cleaner for this stage
	reqQuery := &jmap.Request{}
	q := &email.Query{
		Account: c.getMailAccountID(),
		Filter: &email.FilterCondition{
			InMailbox: jmap.ID(mailboxID),
		},
		Sort: []*email.SortComparator{
			{Property: "receivedAt", IsAscending: false},
		},
		Limit:    limit,
		Position: int64(position),
	}
	reqQuery.Invoke(q)

	resp1, err := c.Client.Do(reqQuery)
	if err != nil {
		return nil, fmt.Errorf("Email/query failed: %w", err)
	}

	var ids []jmap.ID
	for _, inv := range resp1.Responses {
		if res, ok := inv.Args.(*email.QueryResponse); ok {
			ids = res.IDs
		}
	}

	if len(ids) == 0 {
		return []model.Email{}, nil
	}

	// 2. Email/get
	reqGet := &jmap.Request{}
	g := &email.Get{
		Account:    c.getMailAccountID(),
		IDs:        ids,
		Properties: []string{"id", "subject", "from", "to", "cc", "bcc", "replyTo", "preview", "receivedAt", "mailboxIds", "threadId", "keywords"},
	}
	reqGet.Invoke(g)

	resp2, err := c.Client.Do(reqGet)
	if err != nil {
		return nil, fmt.Errorf("Email/get failed: %w", err)
	}

	for _, inv := range resp2.Responses {
		if res, ok := inv.Args.(*email.GetResponse); ok {
			for _, e := range res.List {
				// Convert to model.Email
				sender := formatAddresses(e.From)
				to := formatAddresses(e.To)
				cc := formatAddresses(e.CC)
				bcc := formatAddresses(e.BCC)
				replyTo := formatAddresses(e.ReplyTo)

				isUnread := true
				if _, ok := e.Keywords["$seen"]; ok {
					isUnread = false
				}

				var boxIDs []string
				for k := range e.MailboxIDs {
					boxIDs = append(boxIDs, string(k))
				}

				dateStr := ""
				if e.ReceivedAt != nil {
					dateStr = e.ReceivedAt.Format("2006-01-02 15:04")
				}

				emails = append(emails, model.Email{
					ID:         string(e.ID),
					Subject:    e.Subject,
					From:       sender,
					To:         to,
					Cc:         cc,
					Bcc:        bcc,
					ReplyTo:    replyTo,
					Preview:    e.Preview,
					Date:       dateStr,
					IsUnread:   isUnread,
					ThreadID:   string(e.ThreadID),
					MailboxIDs: boxIDs,
				})
			}
		}
	}
	return emails, nil
}

// FetchEmailBody fetches the full text body for a specific email ID.
func (c *Client) FetchEmailBody(emailID string) (string, error) {
	req := &jmap.Request{}
	g := &email.Get{
		Account:             c.getMailAccountID(),
		IDs:                 []jmap.ID{jmap.ID(emailID)},
		Properties:          []string{"bodyValues", "textBody", "htmlBody"},
		FetchTextBodyValues: true,
		FetchHTMLBodyValues: true,
	}
	req.Invoke(g)

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Email/get failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if res, ok := inv.Args.(*email.GetResponse); ok {
			if len(res.List) > 0 {
				e := res.List[0]

				// 1. Try Plain Text
				for _, part := range e.TextBody {
					if val, ok := e.BodyValues[part.PartID]; ok {
						return val.Value, nil
					}
				}

				// 2. Try HTML
				for _, part := range e.HTMLBody {
					if val, ok := e.BodyValues[part.PartID]; ok {
						converter := md.NewConverter("", true, nil)
						text, err := converter.ConvertString(val.Value)
						if err != nil {
							// Fallback to raw HTML (well, partial)
							return "[HTML Convert Error] " + val.Value, nil
						}
						// Add a header to indicate converted content
						return "[Converted HTML]\n" + text, nil
					}
				}

				return "(No text or html body found)", nil
			}
		}
	}
	return "", fmt.Errorf("email not found")
}

// GetMailboxIDByRole finds a mailbox ID by its role (e.g., "drafts", "sent").
func (c *Client) GetMailboxIDByRole(role string) (string, error) {
mbs, err := c.FetchMailboxes()
if err != nil {
return "", err
}
for _, mb := range mbs {
if mb.Role == role {
return mb.ID, nil
}
}
return "", fmt.Errorf("mailbox with role %s not found", role)
}

// SendEmail creates a draft and submits it.
func (c *Client) SendEmail(from, to, subject, body string) error {	if from == "" {
		if c.Session != nil && c.Session.Username != "" {
			from = c.Session.Username
		} else {
			return fmt.Errorf("sender address (from) is required and could not be determined from session")
		}
	}
draftsID, err := c.GetMailboxIDByRole("drafts")
if err != nil {
// Fallback: try to proceed without specific mailbox or fail?
// JMAP allows creating email without mailboxIds, it just won't be visible in folders.
// But usually we want it in Drafts during creation.
return fmt.Errorf("could not find Drafts folder: %w", err)
}

// 1. Prepare Email Object
draftID := jmap.ID("draft-0")
emailObj := &email.Email{
From:    []*mail.Address{{Name: from, Email: from}},
To:      []*mail.Address{{Name: to, Email: to}},
Subject: subject,
BodyStructure: &email.BodyPart{
PartID: "text",
Type:   "text/plain",
},
BodyValues: map[string]*email.BodyValue{
"text": {Value: body},
},
MailboxIDs: map[jmap.ID]bool{jmap.ID(draftsID): true},
Keywords:   map[string]bool{"$draft": true},
}

// 2. Prepare Submission Object
submitID := jmap.ID("submit-0")
submissionObj := &emailsubmission.EmailSubmission{
EmailID: jmap.ID("#" + string(draftID)),
Envelope: &emailsubmission.Envelope{
MailFrom: &emailsubmission.Address{Email: from},
RcptTo:   []*emailsubmission.Address{{Email: to}},
},
}

// 3. Chain Requests
req := &jmap.Request{}

// Email/set
req.Invoke(&email.Set{
		Account: c.getMailAccountID(),
		Create: map[jmap.ID]*email.Email{
			draftID: emailObj,
		},
	})

	// EmailSubmission/set
	req.Invoke(&emailsubmission.Set{
		Account: c.getMailAccountID(),
		Create: map[jmap.ID]*emailsubmission.EmailSubmission{
			submitID: submissionObj,
		},
		OnSuccessUpdateEmail: map[jmap.ID]jmap.Patch{
			jmap.ID("#" + string(submitID)): map[string]interface{}{
				"mailboxIds/" + draftsID: nil, // Remove from Drafts upon success
			},
		},
	})

resp, err := c.Client.Do(req)
if err != nil {
return fmt.Errorf("JMAP request failed: %w", err)
}

// Check response for errors
for _, inv := range resp.Responses {
if methodErr, ok := inv.Args.(*jmap.MethodError); ok {
return fmt.Errorf("method error in %s: %s", inv.Name, methodErr.Type)
}
// Also check SetResponse for NotCreated
if setResp, ok := inv.Args.(*email.SetResponse); ok {
if len(setResp.NotCreated) > 0 {
return fmt.Errorf("failed to create email: %v", setResp.NotCreated)
}
}
if subResp, ok := inv.Args.(*emailsubmission.SetResponse); ok {
if len(subResp.NotCreated) > 0 {
return fmt.Errorf("failed to submit email: %v", subResp.NotCreated)
}
}
}

return nil
}

func formatAddresses(addrs []*mail.Address) string {
var parts []string
for _, a := range addrs {
if a.Name != "" {
parts = append(parts, fmt.Sprintf("%s <%s>", a.Name, a.Email))
} else {
parts = append(parts, a.Email)
}
}
return strings.Join(parts, ", ")
}

