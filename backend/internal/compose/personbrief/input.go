// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// What one brief is written from, and the fingerprint that decides whether a
// cached one still describes it.
//
// Nothing here re-queries. Every field is folded out of the Person360 the
// caller already assembled, which is what makes the brief's scope exactly the
// reader's own scope without a second set of gates to keep in agreement.
//
// The input leads with what the relationship MEANS and follows with what was
// said. Claims say what was promised, asked and objected to; the changes say
// what moved; the moment says what the fixed ladder thinks is due. The timeline
// comes last and carries the server's own one-line summary of each message, not
// its subject alone — a brief written from subjects and directions can say no
// more than that mail was exchanged, which is the failure this ordering exists
// to stop.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/personcontext"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// floorVersion is bumped by hand when the DETERMINISTIC floor's wording
// changes.
//
// The floor's output is cached like the model's, and its sentences are built by
// Go code — `fmt.Sprintf` formats inside deterministic.go — so there is nothing
// to hash. A deploy that rewords the floor and leaves this alone keeps serving
// the old sentences to every contact whose facts have not moved, which is most
// of them.
//
// It covers ONLY the floor. The model prompt versions itself below.
const floorVersion = "person-brief-floor-v2"

// promptVersion is DERIVED from the prompt as it is SENT — boundary rule
// included — so editing that wording bumps it whether or not anybody remembers
// to.
//
// Digested at ONE fixed language, the way orgbrief, orgdossier and dealstatus
// do it. The language is its own component of Fingerprint below, so folding it
// in here would say the same thing twice — and this is a package-level var
// computed at init, where no installation's setting is readable at all. What it
// captures is the WORDING, which English captures completely.
var promptVersion = ai.PromptDigest(func(fence promptfence.Fence) string {
	return briefSystemFor(fence, string(textlang.English))
})

// briefInputActivities bounds the timeline the brief reads. Each row now
// carries a line of what was actually written, so six of them say more than the
// ten subjects they replace — and a brief is four or five sentences, so a longer
// window buys nothing a reader will see while making the fingerprint churn on
// activity that never changes the text.
const briefInputActivities = 6

// briefInputClaims bounds the claims the brief reads, newest first. The
// commitments card renders them all — the brief only needs enough to say what
// this person cares about and what is outstanding.
const briefInputClaims = 8

// briefInputChanges bounds what CHANGED. The 360 orders these most consequential
// first, and a brief that listed every one of them would be reporting history
// where the reader asked what moved.
const briefInputChanges = 3

// Input is what one brief is written from: who this person is commercially,
// what has been said in conversation with them, what moved, and what recently
// happened — each already pruned to the reader's row scope by the read that
// produced it.
type Input struct {
	Name         string `json:"name"`
	Title        string `json:"title,omitempty"`
	Employer     string `json:"employer,omitempty"`
	BuyingRole   string `json:"buying_role,omitempty"`
	Strength     int    `json:"strength"`
	LastInbound  string `json:"last_inbound,omitempty"`
	LastOutbound string `json:"last_outbound,omitempty"`

	OpenDeal *DealIn    `json:"open_deal,omitempty"`
	Moment   *MomentIn  `json:"moment,omitempty"`
	Changes  []ChangeIn `json:"changes,omitempty"`
	Claims   []ClaimIn  `json:"claims,omitempty"`
	Recent   []ActIn    `json:"recent,omitempty"`

	// SectionsOmitted names what the reader could NOT see. It rides the
	// fingerprint so two readers with different grants never share a cached
	// brief, and it tells the writer to stay silent about those sections
	// rather than inferring around the gap.
	SectionsOmitted []string `json:"sections_omitted,omitempty"`
}

// DealIn is the open deal this person sits on, as the brief reads it.
type DealIn struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stage string `json:"stage,omitempty"`
	// AmountMinor is the exact integer the column holds and what this package
	// does arithmetic on. It does NOT reach the model: MarshalJSON renders
	// `amount` from it in MAJOR units, because a prompt carrying minor units
	// once had a model read a 180,000 EUR deal as eighteen million and write
	// that onto a screen whose own card said 180,000. Dividing by 100 at the
	// point of use is wrong too — a zero-decimal currency has no minor unit —
	// so values.MajorUnits carries the ISO 4217 table.
	AmountMinor int64  `json:"-"`
	Currency    string `json:"currency,omitempty"`
	CloseDate   string `json:"close_date,omitempty"`
}

// MarshalJSON renders the deal's amount as a person would say it, derived from
// the integer at the moment it is written. Two spellings of one number that a
// caller can set independently are two numbers.
func (d DealIn) MarshalJSON() ([]byte, error) {
	type plain DealIn
	return json.Marshal(struct {
		plain
		Amount string `json:"amount,omitempty"`
	}{plain: plain(d), Amount: renderedAmount(d.AmountMinor, d.Currency)})
}

func renderedAmount(minor int64, currency string) string {
	if minor == 0 || currency == "" {
		return ""
	}
	return values.MajorUnits(minor, currency)
}

// ClaimIn is one thing this person said, as the brief reads it.
//
// The kind rides along because "she objected to X" and "she asked for X" are
// opposite claims about the same sentence, and the body alone loses which one
// it was. So does the status: a commitment that is still open and one that was
// kept read identically as bodies, and only one of them is something to do.
type ClaimIn struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Body   string `json:"body"`
	Status string `json:"status"`
	DueAt  string `json:"due_at,omitempty"`
	// Quote is the verbatim excerpt the claim was read from — the contact's own
	// words. A brief that can quote what somebody actually wrote says something
	// a template never could, and the reader can check it against the source.
	Quote string `json:"quote,omitempty"`
	// SourceID is the activity the claim was read from — carried so a sentence
	// about a claim can cite the conversation rather than the derived row.
	SourceID string `json:"source_id"`
}

// ChangeIn is one thing that CHANGED about the relationship. Derived at read
// from the person's own interactions, so it names no row of its own and is not
// citable; a sentence drawn from one cites the person it is about.
type ChangeIn struct {
	Kind string `json:"kind"`
	At   string `json:"at"`
	Days int    `json:"days,omitempty"`
	From string `json:"from_band,omitempty"`
	To   string `json:"to_band,omitempty"`
}

// MomentIn is the ONE thing this contact needs today, as the fixed server-side
// ladder selected it.
//
// The headline is written from the evidence by that ladder and is carried
// verbatim: it is already the honest sentence, and the brief's job is to place
// it against everything else rather than to paraphrase it.
type MomentIn struct {
	Rule     string `json:"rule"`
	Headline string `json:"headline"`
	// Sources are the activity rows the moment fired on, so a sentence about it
	// cites what a reader can open. Empty when the moment rests on derived
	// facts alone.
	Sources []string `json:"source_activity_ids,omitempty"`
}

// ActIn is one recent timeline item as the brief reads it.
//
// Preview is the server's own one line of the sender's text — signature and
// quoted history already stripped — and it is the field that decides whether
// this brief can say anything. A row reduced to kind, subject and direction
// supports exactly one sentence ("they emailed you about X"), which is the
// generic prose the model lane exists to replace.
//
// Withheld says the row is one the reader may not read. It is carried rather
// than dropped because the DATE is theirs even when the words are not: a brief
// that silently omitted a held message would tell a reader nobody had written
// in a fortnight when somebody had. Subject and Preview are already empty on
// such a row — the 360 nulls them before this fold ever sees them — so the flag
// adds no content, it explains an absence.
type ActIn struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Subject   string `json:"subject,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Direction string `json:"direction,omitempty"`
	// Move is the server's reading of whose turn it is on this message:
	// `needs_reply`, `waiting_for_them`, or `none` when the question cannot be
	// answered honestly. Empty on a row that is not mail.
	Move     string `json:"move,omitempty"`
	Withheld bool   `json:"withheld,omitempty"`
	At       string `json:"at"`
}

// FromView folds the assembled 360 into the brief's input. Nothing re-queries:
// the reader's scope is already spent, and asking again would be a second set
// of gates that could disagree with the first.
func FromView(view crmcontracts.Person360) Input {
	in := Input{
		Name:            view.Person.FullName,
		SectionsOmitted: omittedNames(view.SectionsOmitted),
	}
	if view.Person.Title != nil {
		in.Title = *view.Person.Title
	}
	if view.Strength != nil {
		in.Strength = view.Strength.Score
	}
	in.LastInbound = stamp(view.LastInboundAt)
	in.LastOutbound = stamp(view.LastOutboundAt)
	in.Employer = currentEmployer(view)
	foldCommercial(&in, view)
	foldMoment(&in, view)
	foldChanges(&in, view)
	foldClaims(&in, view)
	foldRecent(&in, view)
	return in
}

// omittedNames renders the withheld sections as the plain strings the
// fingerprint hashes and the writer reads. The contract types them as an enum;
// the brief only needs to know which names are in the list.
func omittedNames(omitted []crmcontracts.Person360SectionsOmitted) []string {
	return personcontext.OmittedNames(omitted)
}

// currentEmployer names where this person works now. The 360 sorts the
// current-primary employment to index zero, so the first row is the answer.
func currentEmployer(view crmcontracts.Person360) string { return personcontext.CurrentEmployer(view) }

func foldCommercial(in *Input, view crmcontracts.Person360) {
	if view.Commercial == nil {
		return
	}
	if view.Commercial.Role != nil {
		in.BuyingRole = *view.Commercial.Role
	}
	deal := view.Commercial.Deal
	if deal == nil {
		return
	}
	folded := DealIn{ID: deal.DealId.String(), Name: deal.Title}
	if deal.Stage != nil {
		folded.Stage = *deal.Stage
	}
	if deal.AmountMinor != nil {
		folded.AmountMinor = *deal.AmountMinor
	}
	if deal.Currency != nil {
		folded.Currency = *deal.Currency
	}
	if deal.CloseDate != nil {
		folded.CloseDate = deal.CloseDate.String()
	}
	in.OpenDeal = &folded
}

func foldClaims(in *Input, view crmcontracts.Person360) {
	if view.Claims == nil {
		return
	}
	for _, claim := range *view.Claims {
		if len(in.Claims) == briefInputClaims {
			break
		}
		if claim.Status == crmcontracts.ConversationClaimStatusDismissed {
			// A dismissed claim is one a human said was never true. Writing a
			// brief from it would resurrect it in prose.
			continue
		}
		folded := ClaimIn{
			ID:       claim.Id.String(),
			Kind:     string(claim.Kind),
			Body:     claim.Body,
			Status:   string(claim.Status),
			Quote:    claim.SourceQuote,
			SourceID: claim.SourceActivityId.String(),
		}
		if claim.DueAt != nil {
			folded.DueAt = stampAt(*claim.DueAt)
		}
		in.Claims = append(in.Claims, folded)
	}
}

func foldChanges(in *Input, view crmcontracts.Person360) {
	if view.RelationshipChanges == nil {
		return
	}
	for _, change := range *view.RelationshipChanges {
		if len(in.Changes) == briefInputChanges {
			break
		}
		folded := ChangeIn{Kind: string(change.Kind), At: stampAt(change.At)}
		if change.Days != nil {
			folded.Days = *change.Days
		}
		if change.FromBucket != nil {
			folded.From = string(*change.FromBucket)
		}
		if change.ToBucket != nil {
			folded.To = string(*change.ToBucket)
		}
		in.Changes = append(in.Changes, folded)
	}
}

// foldMoment carries the ladder's selected moment and the activities behind it.
// Evidence of kind relationship_change names no row, so only the activity and
// task rows — both of which ARE activities in this system — become citations.
func foldMoment(in *Input, view crmcontracts.Person360) {
	if view.Moment == nil {
		return
	}
	folded := MomentIn{Rule: string(view.Moment.Rule), Headline: view.Moment.Headline}
	for _, cited := range view.Moment.Evidence {
		if cited.Id == nil {
			continue
		}
		switch cited.Type {
		case crmcontracts.PersonMomentEvidenceTypeActivity, crmcontracts.PersonMomentEvidenceTypeTask:
			folded.Sources = append(folded.Sources, cited.Id.String())
		case crmcontracts.PersonMomentEvidenceTypeRelationshipChange:
		}
	}
	in.Moment = &folded
}

func foldRecent(in *Input, view crmcontracts.Person360) {
	if view.Activities == nil {
		return
	}
	for _, activity := range view.Activities.Data {
		if len(in.Recent) == briefInputActivities {
			break
		}
		folded := ActIn{
			ID:       activity.Id.String(),
			Kind:     string(activity.Kind),
			At:       stampAt(activity.OccurredAt),
			Withheld: withheldContent(activity),
		}
		if activity.Subject != nil {
			folded.Subject = *activity.Subject
		}
		if activity.Direction != nil {
			folded.Direction = string(*activity.Direction)
		}
		foldMailSummary(&folded, activity.EmailSummary)
		in.Recent = append(in.Recent, folded)
	}
}

// foldMailSummary takes what the server already worked out about one message:
// the line of text it is about, and whose move it is.
//
// Read from the projection rather than from the activity's own body, because
// that projection is the one place the stripping of signatures and quoted
// history is spelled, and a second reading of the body here would be a second
// answer to the question the timeline card already answers.
func foldMailSummary(into *ActIn, summary *crmcontracts.EmailSummary) {
	if summary == nil {
		return
	}
	if summary.Preview != nil {
		into.Preview = *summary.Preview
	}
	if summary.Move != crmcontracts.EmailSummaryMoveNone {
		into.Move = string(summary.Move)
	}
}

// withheldContent reports a row whose words are not this reader's. The 360 has
// already nulled its subject and body; this only names why they are missing.
func withheldContent(activity crmcontracts.Activity) bool {
	return activity.ContentState != nil &&
		*activity.ContentState == crmcontracts.ActivityContentStateWithheld
}

// stamp renders an optional instant in one fixed format, so two timestamps
// compare as strings the way the instants they name compare — and so the
// fingerprint does not churn on a formatting difference.
func stamp(at *time.Time) string { return personcontext.Stamp(at) }

// stampAt is stamp for an instant that is always present.
func stampAt(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// Fingerprint keys the cache on everything that could change the text: the
// assembled input, the floor's and the prompt's versions, the model routing
// version, and the language the brief is written in.
//
// Keying on the person row would serve a brief describing a relationship that
// has since moved — activities, claims and conversations change without
// touching it. routingVersion folds in the model binding, so re-pointing a lane
// rewrites briefs instead of leaving text attributed to a model that no longer
// writes it. The LANGUAGE is a component of its own: the brief is written in
// it, and nothing else about the person moves when an installation changes it.
func Fingerprint(in Input, routingVersion, lang string) (string, error) {
	// json.Marshal orders struct fields by declaration, so the same input
	// hashes the same way across processes — a map would not.
	encoded, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("fingerprint the brief input: %w", err)
	}
	sum := sha256.Sum256([]byte(floorVersion + "\x00" + promptVersion + "\x00" +
		routingVersion + "\x00" + lang + "\x00" + string(encoded)))
	return hex.EncodeToString(sum[:]), nil
}
