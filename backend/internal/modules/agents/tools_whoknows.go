// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The who_knows tool: which colleagues know one contact, warmest first.
//
// Its own file because tools_network.go holds four tools and crossed the length
// a reader can keep in their head at once. The seam type, the answer and the
// handler travel together — they are one question and change together.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// WhoKnowsLister answers "which colleagues know this contact", warmest first.
// Compose implements it over the interaction projection through the same
// row-scoped read the HTTP surface uses.
// The bool is truncation, spelled the way IntroPathLister spells it: the walk is
// capped, and a capped list a model is handed with nothing marking it is one it
// will report as the whole network.
type WhoKnowsLister func(ctx context.Context, contactID ids.UUID) (WhoKnowsReading, error)

// WhoKnowsReading is one read of the network around a contact: who knows them,
// whether the walk was cut short, and the contact's own name.
//
// A struct rather than three returns, the shape CoverageReader already uses,
// because the name arrived after the other two and a fourth positional result
// is the kind of signature a caller gets wrong silently.
type WhoKnowsReading struct {
	// ContactName is the anchor's own name. Empty where the workspace holds
	// none, or where row scope withheld the row the name would come from — the
	// name read names what it can and says nothing about the rest, so an empty
	// string here is "no name to show", never "a name was denied".
	ContactName string
	Colleagues  []KnownColleague
	Truncated   bool
}

// WhoKnowsAnswer is what who_knows answers: the colleagues who know one
// contact, warmest first. An empty list is a real answer — it says the contact
// is cold — so it is never an error and never null.
type WhoKnowsAnswer struct {
	ContactID ids.UUID `json:"contact_id"`
	// ContactName is who the question was about. The sibling field on
	// CoverageSeat argues the same case: an answer about a bare uuid has not
	// told a rep which human it concerns, and a surface with no name to print
	// prints the id instead.
	//
	// Omitted rather than empty where there is no name, so a client renders
	// nothing rather than a blank where a name belongs.
	ContactName string           `json:"contact_name,omitempty"`
	Colleagues  []KnownColleague `json:"colleagues"`
}

// --- who_knows (🟢 read) ---

type whoKnowsTool struct{ list WhoKnowsLister }

func (t whoKnowsTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "who_knows", Title: "Possible duplicate", Version: toolVersionV1,
		Description:   whoKnowsCopy.render(),
		Instead:       whoKnowsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getContactNetwork",
		InputSchema: schema(`{"type":"object","properties":{
			"contact_id":{"type":"string","format":"uuid","description":"The contact to ask about"}},
			"required":["contact_id"],"additionalProperties":false}`),
		OutputSchema: schemaFor[WhoKnowsAnswer](),
	}
}

// whoKnowsTruncatedMessage is the third spelling of one rule: a ranked list that
// stopped at its cap is not the whole network, and a model told nothing reports
// it as one.
//
// It has to be true of BOTH bounds the seam reports through one flag, and they
// differ in what they cost: the result cap trims a full ranking, so the ten
// returned really are the warmest, while the scan bound stops the reading before
// the ranking, so past it they are the warmest of a sample. A message asserting
// either would be false half the time — so it states what holds in both cases,
// which is that colleagues exist beyond this list and it is not the network.
const whoKnowsTruncatedMessage = "More colleagues know this contact than are listed here. " +
	"Report these as the ones found, not as everyone who knows them."

func (t whoKnowsTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		ContactID ids.UUID `json:"contact_id"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	reading, err := t.list(ctx, args.ContactID)
	if err != nil {
		return nil, err
	}
	colleagues, truncated := reading.Colleagues, reading.Truncated
	if colleagues == nil {
		// An empty LIST, not a null. The documented shape is an array, and a
		// model handed null reads it as "unknown" rather than "nobody".
		colleagues = []KnownColleague{}
	}
	// An empty answer is returned as an empty list, not an error. "Nobody here
	// knows them" is a true and useful answer to this question — it is the
	// answer that says the account is cold — and turning it into a failure
	// would make the model narrate a problem instead of a fact.
	noteDerivedContent(ctx)
	noteEvidence(ctx, datasource.EntityContact, args.ContactID)
	if truncated {
		noteWarning(ctx, warningSweepTruncated, whoKnowsTruncatedMessage)
	}
	return json.Marshal(WhoKnowsAnswer{
		ContactID: args.ContactID, ContactName: reading.ContactName, Colleagues: colleagues,
	})
}
