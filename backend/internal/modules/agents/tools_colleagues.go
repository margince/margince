// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// list_colleagues (🟢 read): who works here, as opposed to who we sell to.
//
// The surface had no way to name a colleague. app_user appeared in none of the
// tools, so an assistant asked to assign work searched `contact` — the customer
// contacts — found two contacts with the right first name, and reported that
// neither was "listed under sales". The seat it wanted was the one the human
// was signed in as.
//
// That also blocked everything downstream: assignee_id and owner_id take an
// app_user id, and nothing could produce one.
//
// NOT a record type. A seat has no owner, no visibility rule and no custom
// fields, and datasource.EntityTypes is pinned to the schema's CHECK
// constraints — widening it for this would ripple through every polymorphic
// reference to say "a colleague is a kind of record", which is exactly the
// confusion this tool exists to end.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Colleague is one workspace seat.
type Colleague struct {
	UserID      ids.UUID `json:"user_id"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email"`
	SeatType    string   `json:"seat_type"`
	IsAgent     bool     `json:"is_agent"`
}

// ColleagueLister answers the roster. Declared here, implemented in compose,
// so this module never imports identity.
type ColleagueLister func(ctx context.Context, q string) (colleagues []Colleague, truncated bool, err error)

// RegisterColleaguesTool joins list_colleagues to the surface; a nil lister
// registers nothing.
func RegisterColleaguesTool(r *Registry, list ColleagueLister) {
	if list == nil {
		return
	}
	r.Register(listColleagues{list: list})
}

type listColleagues struct{ list ColleagueLister }

func (t listColleagues) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_colleagues", Title: "List colleagues", Version: toolVersionV1,
		Description:   listColleaguesCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "listUsers",
		InputSchema: schema(`{"type":"object","properties":{
			"q":{"type":"string","description":"Narrow by name or email; omit for the whole roster"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ListColleaguesResult](),
	}
}

func (t listColleagues) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Q string `json:"q"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	colleagues, truncated, err := t.list(ctx, args.Q)
	if err != nil {
		return nil, err
	}
	// No noteEvidence: a seat is not a record this answer rests on. Stamping
	// one would put a colleague in the evidence list of every call that begins
	// by asking who could do the work.
	if colleagues == nil {
		colleagues = []Colleague{}
	}
	out := ListColleaguesResult{Colleagues: colleagues, Truncated: truncated}
	if len(colleagues) == 0 && strings.TrimSpace(args.Q) != "" {
		// TrimSpace because the roster read trims before deciding whether a
		// narrowing was asked for at all; two spellings of "the caller narrowed"
		// would disagree on `q` of "   ".
		attachEveryColleague(ctx, t.list, &out)
	}
	return json.Marshal(out)
}

// CodeRosterUnreadable says the fallback roster could not be read, so an absent
// `all_colleagues` is unknown rather than empty.
const CodeRosterUnreadable = "colleague_roster_unreadable"

// CodeNameMatchedNoColleague says the narrowing matched nobody and names what
// the answer carries instead.
const CodeNameMatchedNoColleague = "name_matched_no_colleague"

// attachEveryColleague hands over the set the narrowing was matched against,
// and says in the envelope what happened — the channel a degraded answer uses
// here already (CodeSemanticRankingDegraded, CodeDuplicateCheckFailed), which
// costs nothing until the miss and keeps the standing tool copy short.
//
// It returns nothing and cannot fail the call, for the reason reportDuplicates
// gives: the question that was asked has already been answered correctly, and
// throwing that away because a courtesy read failed hands the caller a tool
// failure where it had a true answer.
func attachEveryColleague(ctx context.Context, list ColleagueLister, out *ListColleaguesResult) {
	everyone, truncated, err := list(ctx, "")
	if err != nil {
		noteWarning(ctx, CodeRosterUnreadable,
			"Nobody here is spelled that way. The rest of the roster could not be read to show "+
				"you who is, so treat this as unknown rather than as an empty workspace.")
		return
	}
	if everyone == nil {
		everyone = []Colleague{}
	}
	out.AllColleagues, out.AllColleaguesTruncated = &everyone, truncated
	if len(everyone) == 0 {
		noteWarning(ctx, CodeNameMatchedNoColleague,
			"This workspace has no colleagues who can receive work.")
		return
	}
	if truncated {
		noteWarning(ctx, CodeNameMatchedNoColleague,
			"Nobody here is spelled that way. `all_colleagues` is a CAPPED page of the roster, "+
				"not all of it, so do not conclude from it that the contact has no seat — narrow "+
				"differently, on a surname or an email fragment.")
		return
	}
	noteWarning(ctx, CodeNameMatchedNoColleague,
		"Nobody here is spelled that way. `all_colleagues` is everyone who can receive work; "+
			"pick from it, or tell the user the contact has no seat.")
}
