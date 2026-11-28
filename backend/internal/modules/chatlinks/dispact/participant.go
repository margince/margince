// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// MaxGroupRecipients bounds explicit selection for a single private group.
const MaxGroupRecipients = 8

// Participant uses a member credential independently of the shared Dispact bot.
type Participant struct {
	http            messaging.HTTP
	workspace, user string
}

// NewParticipant binds Dispact personal operations to a verified native identity.
func NewParticipant(client messaging.Doer, address, token, workspace, user string) *Participant {
	return &Participant{http: messaging.HTTP{Client: client, Base: strings.TrimRight(address, "/") + "/api", Token: token}, workspace: workspace, user: user}
}

var (
	_             messaging.Client = (*Participant)(nil)
	participantID                  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,79}$`)
)

type participantUser struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace_id"`
	Name      string `json:"display_name"`
	Email     string `json:"email"`
	Active    bool   `json:"is_active"`
	Guest     bool   `json:"is_guest"`
	Bot       bool   `json:"is_bot"`
	Pending   bool   `json:"is_import_placeholder"`
}

func (c *Participant) person(user participantUser) messaging.Member {
	return messaging.Member{ID: user.ID, Name: user.Name, Email: user.Email, Active: user.Active && !user.Bot && !user.Pending, External: user.Guest || user.Workspace != c.workspace}
}

// Identity binds personal consent to one provider workspace and sender.
func (c *Participant) Identity(ctx context.Context) (messaging.Identity, error) {
	raw, err := c.http.Call(ctx, http.MethodGet, "/auth/me", nil)
	if err != nil {
		return messaging.Identity{}, err
	}
	var user participantUser
	if json.Unmarshal(raw, &user) != nil || user.ID == "" || user.Workspace == "" || user.Workspace != c.workspace || c.user != "" && user.ID != c.user {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	member := c.person(user)
	if !member.Active || member.External {
		return messaging.Identity{}, messaging.ErrUnauthorized
	}
	return messaging.Identity{Workspace: user.Workspace, Member: member}, nil
}

func (c *Participant) member(ctx context.Context, id string) (messaging.Member, error) {
	if !participantID.MatchString(id) {
		return messaging.Member{}, messaging.ErrInvalid
	}
	raw, err := c.http.Call(ctx, http.MethodGet, "/users/"+url.PathEscape(id), nil)
	if err != nil {
		return messaging.Member{}, err
	}
	var user participantUser
	if json.Unmarshal(raw, &user) != nil || user.ID != id {
		return messaging.Member{}, messaging.ErrUnavailable
	}
	return c.person(user), nil
}

// People searches native identities through the current participant credential.
func (c *Participant) People(ctx context.Context, query, cursor string) ([]messaging.Member, string, error) {
	offset := 0
	if cursor != "" {
		var err error
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 || offset > 1000000 {
			return nil, "", &storekit.MalformedCursorError{}
		}
	}
	values := url.Values{"workspace_id": {c.workspace}, "q": {query}, limitKey: {"50"}, "offset": {strconv.Itoa(offset)}}
	raw, err := c.http.Call(ctx, http.MethodGet, "/users?"+values.Encode(), nil)
	if err != nil {
		return nil, "", err
	}
	var users []participantUser
	if json.Unmarshal(raw, &users) != nil {
		return nil, "", messaging.ErrUnavailable
	}
	people := []messaging.Member{}
	for _, user := range users {
		member := c.person(user)
		if member.ID != "" && member.Active && !member.External {
			people = append(people, member)
		}
	}
	next := ""
	if len(users) == 50 {
		next = strconv.Itoa(offset + len(users))
	}
	return people, next, nil
}

// ResolvePerson resolves an address to the provider identity used for recipient verification.
func (c *Participant) ResolvePerson(ctx context.Context, email string) (messaging.Member, error) {
	cursor := ""
	var found *messaging.Member
	for range 20 {
		people, next, err := c.People(ctx, email, cursor)
		if err != nil {
			return messaging.Member{}, err
		}
		for _, person := range people {
			if strings.EqualFold(person.Email, email) {
				if found != nil {
					return messaging.Member{}, messaging.ErrInvalid
				}
				found = &person
			}
		}
		if next == "" {
			if found == nil {
				return messaging.Member{}, messaging.ErrUnavailable
			}
			return *found, nil
		}
		cursor = next
	}
	return messaging.Member{}, messaging.ErrUnavailable
}
