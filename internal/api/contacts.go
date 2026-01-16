package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"fm-cli/internal/model"

	"git.sr.ht/~rockorager/go-jmap"
)

// JMAP Contacts capability URI
const ContactsURI jmap.URI = "urn:ietf:params:jmap:contacts"

// ContactCard JMAP types for raw requests
type addressBookGetRequest struct {
	AccountID string `json:"accountId"`
}

type contactCardGetRequest struct {
	AccountID  string   `json:"accountId"`
	IDs        []string `json:"ids,omitempty"`
	Properties []string `json:"properties,omitempty"`
}

type contactCardQueryRequest struct {
	AccountID string                      `json:"accountId"`
	Filter    *contactCardFilterCondition `json:"filter,omitempty"`
	Sort      []contactCardSort           `json:"sort,omitempty"`
	Position  int                         `json:"position,omitempty"`
	Limit     int                         `json:"limit,omitempty"`
}

type contactCardFilterCondition struct {
	InAddressBook string `json:"inAddressBook,omitempty"`
	Text          string `json:"text,omitempty"`
}

type contactCardSort struct {
	Property    string `json:"property"`
	IsAscending bool   `json:"isAscending"`
}

type contactCardSetRequest struct {
	AccountID string                     `json:"accountId"`
	Create    map[string]contactCardData `json:"create,omitempty"`
	Update    map[string]contactCardData `json:"update,omitempty"`
	Destroy   []string                   `json:"destroy,omitempty"`
}

type contactCardData struct {
	AddressBookIDs map[string]bool      `json:"addressBookIds,omitempty"`
	Type           string               `json:"@type,omitempty"`
	Name           jsContactName        `json:"name,omitempty"`
	Nicknames      map[string]jsNickname `json:"nicknames,omitempty"`
	Organizations  map[string]jsOrg      `json:"organizations,omitempty"`
	Emails         map[string]jsEmail    `json:"emails,omitempty"`
	Phones         map[string]jsPhone    `json:"phones,omitempty"`
	Addresses      map[string]jsAddress  `json:"addresses,omitempty"`
	Notes          string               `json:"notes,omitempty"`
	Anniversaries  map[string]jsDate    `json:"anniversaries,omitempty"`
}

type jsContactName struct {
	Full       string            `json:"full,omitempty"`
	Components []jsNameComponent `json:"components,omitempty"`
}

type jsNameComponent struct {
	Kind  string `json:"kind"` // prefix, given, surname, suffix
	Value string `json:"value"`
}

type jsNickname struct {
	Name string `json:"name"`
}

type jsOrg struct {
	Name string `json:"name"`
}

type jsEmail struct {
	Address  string            `json:"address"`
	Contexts map[string]bool   `json:"contexts,omitempty"` // work, private
	Pref     int               `json:"pref,omitempty"`
}

type jsPhone struct {
	Number   string          `json:"number"`
	Features map[string]bool `json:"features,omitempty"` // voice, cell, fax
	Contexts map[string]bool `json:"contexts,omitempty"` // work, private
	Pref     int             `json:"pref,omitempty"`
}

type jsAddress struct {
	Street     string          `json:"street,omitempty"`
	Locality   string          `json:"locality,omitempty"`
	Region     string          `json:"region,omitempty"`
	PostalCode string          `json:"postcode,omitempty"`
	Country    string          `json:"country,omitempty"`
	Contexts   map[string]bool `json:"contexts,omitempty"`
}

type jsDate struct {
	Kind string `json:"kind"` // birth, wedding
	Date string `json:"date"` // YYYY-MM-DD
}

func (c *Client) getContactsAccountID() jmap.ID {
	if c.Session == nil {
		return ""
	}
	// Check if contacts capability exists
	if id, ok := c.Session.PrimaryAccounts[ContactsURI]; ok {
		return id
	}
	// Fallback to mail account (Fastmail uses same account for both)
	return c.Session.PrimaryAccounts["urn:ietf:params:jmap:mail"]
}

// FetchAddressBooks retrieves all address books
func (c *Client) FetchAddressBooks() ([]model.AddressBook, error) {
	accountID := c.getContactsAccountID()
	if accountID == "" {
		return nil, fmt.Errorf("no contacts account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	reqData := addressBookGetRequest{
		AccountID: string(accountID),
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "AddressBook/get",
		CallID: "a0",
		Args:   reqData,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AddressBook/get failed: %w", err)
	}

	var addressBooks []model.AddressBook
	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return nil, fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "AddressBook/get" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				List []struct {
					ID             string `json:"id"`
					Name           string `json:"name"`
					IsDefault      bool   `json:"isDefault"`
					MayReadItems   bool   `json:"mayReadItems"`
					MayAddItems    bool   `json:"mayAddItems"`
					MayModifyItems bool   `json:"mayModifyItems"`
					MayRemoveItems bool   `json:"mayRemoveItems"`
				} `json:"list"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				continue
			}
			for _, ab := range result.List {
				addressBooks = append(addressBooks, model.AddressBook{
					ID:             ab.ID,
					Name:           ab.Name,
					IsDefault:      ab.IsDefault,
					MayReadItems:   ab.MayReadItems,
					MayAddItems:    ab.MayAddItems,
					MayModifyItems: ab.MayModifyItems,
					MayRemoveItems: ab.MayRemoveItems,
				})
			}
		}
	}

	return addressBooks, nil
}

// FetchContacts retrieves contacts, optionally filtered by address book
func (c *Client) FetchContacts(addressBookID string, search string, limit int) ([]model.Contact, error) {
	accountID := c.getContactsAccountID()
	if accountID == "" {
		return nil, fmt.Errorf("no contacts account found")
	}

	if limit <= 0 {
		limit = 100
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	// Query for contacts
	queryReq := contactCardQueryRequest{
		AccountID: string(accountID),
		Sort: []contactCardSort{
			{Property: "name/full", IsAscending: true},
		},
		Limit: limit,
	}

	if addressBookID != "" || search != "" {
		queryReq.Filter = &contactCardFilterCondition{}
		if addressBookID != "" {
			queryReq.Filter.InAddressBook = addressBookID
		}
		if search != "" {
			queryReq.Filter.Text = search
		}
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "ContactCard/query",
		CallID: "q0",
		Args:   queryReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ContactCard/query failed: %w", err)
	}

	// Get contact IDs from query response
	var contactIDs []string
	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return nil, fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "ContactCard/query" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				IDs []string `json:"ids"`
			}
			json.Unmarshal(data, &result)
			contactIDs = result.IDs
		}
	}

	if len(contactIDs) == 0 {
		return []model.Contact{}, nil
	}

	// Fetch contact details
	req2 := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	getReq := contactCardGetRequest{
		AccountID: string(accountID),
		IDs:       contactIDs,
	}

	req2.Calls = append(req2.Calls, &jmap.Invocation{
		Name:   "ContactCard/get",
		CallID: "g0",
		Args:   getReq,
	})

	resp2, err := c.Client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("ContactCard/get failed: %w", err)
	}

	var contacts []model.Contact
	for _, inv := range resp2.Responses {
		if inv.Name == "ContactCard/get" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				List []struct {
					ID             string          `json:"id"`
					AddressBookIDs map[string]bool `json:"addressBookIds"`
					Name           struct {
						Full       string `json:"full"`
						Components []struct {
							Kind  string `json:"kind"`
							Value string `json:"value"`
						} `json:"components"`
					} `json:"name"`
					Nicknames map[string]struct {
						Name string `json:"name"`
					} `json:"nicknames"`
					Organizations map[string]struct {
						Name string `json:"name"`
					} `json:"organizations"`
					Titles map[string]struct {
						Name string `json:"name"`
					} `json:"titles"`
					Emails map[string]struct {
						Address  string          `json:"address"`
						Contexts map[string]bool `json:"contexts"`
						Pref     int             `json:"pref"`
					} `json:"emails"`
					Phones map[string]struct {
						Number   string          `json:"number"`
						Features map[string]bool `json:"features"`
						Contexts map[string]bool `json:"contexts"`
						Pref     int             `json:"pref"`
					} `json:"phones"`
					Addresses map[string]struct {
						Street     string          `json:"street"`
						Locality   string          `json:"locality"`
						Region     string          `json:"region"`
						PostalCode string          `json:"postcode"`
						Country    string          `json:"country"`
						Contexts   map[string]bool `json:"contexts"`
					} `json:"addresses"`
					Notes         string `json:"notes"`
					Anniversaries map[string]struct {
						Kind string `json:"kind"`
						Date string `json:"date"`
					} `json:"anniversaries"`
					Created string `json:"created"`
					Updated string `json:"updated"`
				} `json:"list"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				continue
			}

			for _, c := range result.List {
				contact := model.Contact{
					ID:       c.ID,
					FullName: c.Name.Full,
					Notes:    c.Notes,
				}

				// Get first address book ID
				for abID := range c.AddressBookIDs {
					contact.AddressBookID = abID
					break
				}

				// Parse name components
				for _, comp := range c.Name.Components {
					switch comp.Kind {
					case "prefix":
						contact.Prefix = comp.Value
					case "given":
						contact.FirstName = comp.Value
					case "surname":
						contact.LastName = comp.Value
					case "suffix":
						contact.Suffix = comp.Value
					}
				}

				// Get nickname
				for _, nick := range c.Nicknames {
					contact.Nickname = nick.Name
					break
				}

				// Get company and title
				for _, org := range c.Organizations {
					contact.Company = org.Name
					break
				}
				for _, title := range c.Titles {
					contact.JobTitle = title.Name
					break
				}

				// Convert emails
				for _, e := range c.Emails {
					emailType := "other"
					if e.Contexts["work"] {
						emailType = "work"
					} else if e.Contexts["private"] {
						emailType = "home"
					}
					contact.Emails = append(contact.Emails, model.ContactEmail{
						Type:      emailType,
						Email:     e.Address,
						IsDefault: e.Pref == 1,
					})
				}

				// Convert phones
				for _, p := range c.Phones {
					phoneType := "other"
					if p.Features["cell"] || p.Features["mobile"] {
						phoneType = "mobile"
					} else if p.Features["fax"] {
						phoneType = "fax"
					} else if p.Contexts["work"] {
						phoneType = "work"
					} else if p.Contexts["private"] {
						phoneType = "home"
					}
					contact.Phones = append(contact.Phones, model.ContactPhone{
						Type:      phoneType,
						Number:    p.Number,
						IsDefault: p.Pref == 1,
					})
				}

				// Convert addresses
				for _, a := range c.Addresses {
					addrType := "other"
					if a.Contexts["work"] {
						addrType = "work"
					} else if a.Contexts["private"] {
						addrType = "home"
					}
					contact.Addresses = append(contact.Addresses, model.ContactAddress{
						Type:       addrType,
						Street:     a.Street,
						City:       a.Locality,
						State:      a.Region,
						PostalCode: a.PostalCode,
						Country:    a.Country,
					})
				}

				// Parse anniversaries
				for _, ann := range c.Anniversaries {
					if ann.Kind == "birth" {
						contact.Birthday = ann.Date
					} else if ann.Kind == "wedding" {
						contact.Anniversary = ann.Date
					}
				}

				// Parse timestamps
				if c.Created != "" {
					contact.Created, _ = time.Parse(time.RFC3339, c.Created)
				}
				if c.Updated != "" {
					contact.Updated, _ = time.Parse(time.RFC3339, c.Updated)
				}

				contacts = append(contacts, contact)
			}
		}
	}

	// Sort contacts by name
	sort.Slice(contacts, func(i, j int) bool {
		return strings.ToLower(contacts[i].FullName) < strings.ToLower(contacts[j].FullName)
	})

	return contacts, nil
}

// CreateContact creates a new contact
func (c *Client) CreateContact(contact model.Contact) (string, error) {
	accountID := c.getContactsAccountID()
	if accountID == "" {
		return "", fmt.Errorf("no contacts account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	contactData := contactCardData{
		AddressBookIDs: map[string]bool{contact.AddressBookID: true},
		Type:           "Card",
	}

	// Build name
	contactData.Name = jsContactName{
		Full: contact.FullName,
	}
	if contact.FirstName != "" || contact.LastName != "" {
		if contact.Prefix != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "prefix", Value: contact.Prefix})
		}
		if contact.FirstName != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "given", Value: contact.FirstName})
		}
		if contact.LastName != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "surname", Value: contact.LastName})
		}
		if contact.Suffix != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "suffix", Value: contact.Suffix})
		}
	}

	// Add nickname
	if contact.Nickname != "" {
		contactData.Nicknames = map[string]jsNickname{
			"n1": {Name: contact.Nickname},
		}
	}

	// Add organization
	if contact.Company != "" {
		contactData.Organizations = map[string]jsOrg{
			"o1": {Name: contact.Company},
		}
	}

	// Add emails
	if len(contact.Emails) > 0 {
		contactData.Emails = make(map[string]jsEmail)
		for i, e := range contact.Emails {
			contexts := map[string]bool{}
			switch e.Type {
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			pref := 0
			if e.IsDefault {
				pref = 1
			}
			contactData.Emails[fmt.Sprintf("e%d", i)] = jsEmail{
				Address:  e.Email,
				Contexts: contexts,
				Pref:     pref,
			}
		}
	}

	// Add phones
	if len(contact.Phones) > 0 {
		contactData.Phones = make(map[string]jsPhone)
		for i, p := range contact.Phones {
			features := map[string]bool{"voice": true}
			contexts := map[string]bool{}
			switch p.Type {
			case "mobile":
				features["cell"] = true
			case "fax":
				features = map[string]bool{"fax": true}
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			pref := 0
			if p.IsDefault {
				pref = 1
			}
			contactData.Phones[fmt.Sprintf("p%d", i)] = jsPhone{
				Number:   p.Number,
				Features: features,
				Contexts: contexts,
				Pref:     pref,
			}
		}
	}

	// Add addresses
	if len(contact.Addresses) > 0 {
		contactData.Addresses = make(map[string]jsAddress)
		for i, a := range contact.Addresses {
			contexts := map[string]bool{}
			switch a.Type {
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			contactData.Addresses[fmt.Sprintf("a%d", i)] = jsAddress{
				Street:     a.Street,
				Locality:   a.City,
				Region:     a.State,
				PostalCode: a.PostalCode,
				Country:    a.Country,
				Contexts:   contexts,
			}
		}
	}

	// Add notes
	contactData.Notes = contact.Notes

	// Add anniversaries
	if contact.Birthday != "" || contact.Anniversary != "" {
		contactData.Anniversaries = make(map[string]jsDate)
		if contact.Birthday != "" {
			contactData.Anniversaries["d1"] = jsDate{Kind: "birth", Date: contact.Birthday}
		}
		if contact.Anniversary != "" {
			contactData.Anniversaries["d2"] = jsDate{Kind: "wedding", Date: contact.Anniversary}
		}
	}

	setReq := contactCardSetRequest{
		AccountID: string(accountID),
		Create: map[string]contactCardData{
			"new-contact": contactData,
		},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "ContactCard/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ContactCard/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return "", fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "ContactCard/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				Created map[string]struct {
					ID string `json:"id"`
				} `json:"created"`
				NotCreated map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notCreated"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotCreated) > 0 {
				for _, err := range result.NotCreated {
					return "", fmt.Errorf("failed to create contact: %s - %s", err.Type, err.Description)
				}
			}
			if created, ok := result.Created["new-contact"]; ok {
				return created.ID, nil
			}
		}
	}

	return "", fmt.Errorf("no contact ID returned")
}

// UpdateContact updates an existing contact
func (c *Client) UpdateContact(contact model.Contact) error {
	accountID := c.getContactsAccountID()
	if accountID == "" {
		return fmt.Errorf("no contacts account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	contactData := contactCardData{}

	// Build name
	contactData.Name = jsContactName{
		Full: contact.FullName,
	}
	if contact.FirstName != "" || contact.LastName != "" {
		if contact.Prefix != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "prefix", Value: contact.Prefix})
		}
		if contact.FirstName != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "given", Value: contact.FirstName})
		}
		if contact.LastName != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "surname", Value: contact.LastName})
		}
		if contact.Suffix != "" {
			contactData.Name.Components = append(contactData.Name.Components, jsNameComponent{Kind: "suffix", Value: contact.Suffix})
		}
	}

	// Add nickname
	if contact.Nickname != "" {
		contactData.Nicknames = map[string]jsNickname{
			"n1": {Name: contact.Nickname},
		}
	}

	// Add organization
	if contact.Company != "" {
		contactData.Organizations = map[string]jsOrg{
			"o1": {Name: contact.Company},
		}
	}

	// Add emails
	if len(contact.Emails) > 0 {
		contactData.Emails = make(map[string]jsEmail)
		for i, e := range contact.Emails {
			contexts := map[string]bool{}
			switch e.Type {
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			pref := 0
			if e.IsDefault {
				pref = 1
			}
			contactData.Emails[fmt.Sprintf("e%d", i)] = jsEmail{
				Address:  e.Email,
				Contexts: contexts,
				Pref:     pref,
			}
		}
	}

	// Add phones
	if len(contact.Phones) > 0 {
		contactData.Phones = make(map[string]jsPhone)
		for i, p := range contact.Phones {
			features := map[string]bool{"voice": true}
			contexts := map[string]bool{}
			switch p.Type {
			case "mobile":
				features["cell"] = true
			case "fax":
				features = map[string]bool{"fax": true}
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			pref := 0
			if p.IsDefault {
				pref = 1
			}
			contactData.Phones[fmt.Sprintf("p%d", i)] = jsPhone{
				Number:   p.Number,
				Features: features,
				Contexts: contexts,
				Pref:     pref,
			}
		}
	}

	// Add addresses
	if len(contact.Addresses) > 0 {
		contactData.Addresses = make(map[string]jsAddress)
		for i, a := range contact.Addresses {
			contexts := map[string]bool{}
			switch a.Type {
			case "work":
				contexts["work"] = true
			case "home":
				contexts["private"] = true
			}
			contactData.Addresses[fmt.Sprintf("a%d", i)] = jsAddress{
				Street:     a.Street,
				Locality:   a.City,
				Region:     a.State,
				PostalCode: a.PostalCode,
				Country:    a.Country,
				Contexts:   contexts,
			}
		}
	}

	// Add notes
	contactData.Notes = contact.Notes

	// Add anniversaries
	if contact.Birthday != "" || contact.Anniversary != "" {
		contactData.Anniversaries = make(map[string]jsDate)
		if contact.Birthday != "" {
			contactData.Anniversaries["d1"] = jsDate{Kind: "birth", Date: contact.Birthday}
		}
		if contact.Anniversary != "" {
			contactData.Anniversaries["d2"] = jsDate{Kind: "wedding", Date: contact.Anniversary}
		}
	}

	setReq := contactCardSetRequest{
		AccountID: string(accountID),
		Update: map[string]contactCardData{
			contact.ID: contactData,
		},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "ContactCard/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("ContactCard/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "ContactCard/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				NotUpdated map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notUpdated"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotUpdated) > 0 {
				for _, err := range result.NotUpdated {
					return fmt.Errorf("failed to update contact: %s - %s", err.Type, err.Description)
				}
			}
		}
	}

	return nil
}

// DeleteContact deletes a contact
func (c *Client) DeleteContact(contactID string) error {
	accountID := c.getContactsAccountID()
	if accountID == "" {
		return fmt.Errorf("no contacts account found")
	}

	req := &jmap.Request{
		Using: []jmap.URI{jmap.CoreURI, ContactsURI},
	}

	setReq := contactCardSetRequest{
		AccountID: string(accountID),
		Destroy:   []string{contactID},
	}

	req.Calls = append(req.Calls, &jmap.Invocation{
		Name:   "ContactCard/set",
		CallID: "s0",
		Args:   setReq,
	})

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("ContactCard/set failed: %w", err)
	}

	for _, inv := range resp.Responses {
		if inv.Name == "error" {
			return fmt.Errorf("JMAP error: %v", inv.Args)
		}
		if inv.Name == "ContactCard/set" {
			data, _ := json.Marshal(inv.Args)
			var result struct {
				NotDestroyed map[string]struct {
					Type        string `json:"type"`
					Description string `json:"description"`
				} `json:"notDestroyed"`
			}
			json.Unmarshal(data, &result)

			if len(result.NotDestroyed) > 0 {
				for _, err := range result.NotDestroyed {
					return fmt.Errorf("failed to delete contact: %s - %s", err.Type, err.Description)
				}
			}
		}
	}

	return nil
}
