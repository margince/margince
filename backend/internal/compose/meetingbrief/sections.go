// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

// The eight sections of ADR-0097 D5, in their fixed order.
//
// Four of them are specified as deterministic (header, attendees, commitments,
// company context) and four as model-written (goal, deal state, risks, talking
// points). No model lane is wired to this surface yet, so all eight are written
// here from the assembled records and `generated_by` says `deterministic`
// rather than passing a composition off as a written brief. When a lane
// arrives, the four M sections gain a writer and this file stays as their
// floor — a workspace with no model gets a plainer brief, not a blank one.
//
// Every sentence cites a record. That is not a nicety of the floor: it is the
// same contract the model sections will be held to, so the card renders and
// behaves identically whichever wrote it.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/contactcontext"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// The citable record types. A brief may only point at things the reader can
// open, and these are the ones a prep surface can open in place.
const (
	citeActivity = "activity"
	citeDeal     = "deal"
	citeContact  = "contact"
)

// The two natures this floor writes. A line that RECOMMENDS an action and one
// that READS a risk out of a record are both labelled, because a reader
// forgives a wrong opinion and does not forgive a wrong fact — the sentences
// that are not plain facts are the ones that must say so. Anything unlabelled
// is a fact, which is the contract's default.
const (
	natureAssessment     = string(crmcontracts.CompanyBriefSentenceNatureAssessment)
	natureRecommendation = string(crmcontracts.CompanyBriefSentenceNatureRecommendation)
)

// The claim kinds this floor reads, bound to the contract enum rather than
// spelled as string literals. A kind renamed upstream then fails to COMPILE
// here, instead of silently emptying the section that reads it — a section that
// quietly stops having anything to say is invisible to every gate.
const (
	kindCommitmentOurs   = string(crmcontracts.ConversationClaimKindCommitmentOurs)
	kindCommitmentTheirs = string(crmcontracts.ConversationClaimKindCommitmentTheirs)
	kindOpenQuestion     = string(crmcontracts.ConversationClaimKindOpenQuestion)
	kindDecision         = string(crmcontracts.ConversationClaimKindDecision)
	kindDecisionProcess  = string(crmcontracts.ConversationClaimKindDecisionProcess)
	kindObjection        = string(crmcontracts.ConversationClaimKindObjection)
	kindPriority         = string(crmcontracts.ConversationClaimKindPriority)
	kindSuccessCriterion = string(crmcontracts.ConversationClaimKindSuccessCriterion)
)

// statusOpen is the claim status the risk and goal rules test. A claim already
// done is not a watch-out and not an ask.
const statusOpen = string(crmcontracts.ConversationClaimStatusOpen)

// specOrder is the order the sections render in: goal and commitments lead
// because burying the ask is the canonical prep failure, and company context
// is last because it is background a reader skims only if they have time.
var specOrder = map[crmcontracts.MeetingBriefSectionKind]int{
	crmcontracts.MeetingBriefSectionKindHeader:         0,
	crmcontracts.MeetingBriefSectionKindGoal:           1,
	crmcontracts.MeetingBriefSectionKindWhatChanged:    2,
	crmcontracts.MeetingBriefSectionKindAttendees:      3,
	crmcontracts.MeetingBriefSectionKindCommitments:    4,
	crmcontracts.MeetingBriefSectionKindDealState:      5,
	crmcontracts.MeetingBriefSectionKindRisks:          6,
	crmcontracts.MeetingBriefSectionKindTalkingPoints:  7,
	crmcontracts.MeetingBriefSectionKindCompanyContext: 8,
}

// specSequence is specOrder read as a list: the same order, for the writer that
// builds sections BY kind rather than sorting the ones it filled. Derived from
// the map above so the two cannot disagree about where a section sits.
var specSequence = orderedKinds()

func orderedKinds() []crmcontracts.MeetingBriefSectionKind {
	out := make([]crmcontracts.MeetingBriefSectionKind, 0, len(specOrder))
	for kind := range specOrder {
		out = append(out, kind)
	}
	sort.Slice(out, func(i, j int) bool { return specOrder[out[i]] < specOrder[out[j]] })
	return out
}

// Section is one heading with its lines, before the grounding filter runs.
type Section struct {
	Kind      crmcontracts.MeetingBriefSectionKind
	Sentences []Sentence
}

// Deterministic writes all nine sections from the assembled input alone.
//
// The order is the spec's and is not a rendering choice: goal and commitments
// lead because burying the ask is the canonical prep failure, and company
// context is last because it is background a reader skims only if they have
// time.
func Deterministic(in Input) []Section {
	// One ranked claim set, consumed in section order: the goal takes the
	// sharpest ask, risks take what is wrong, deal state takes what was
	// settled, and talking points get what nobody else said. The commitments
	// ledger reads the whole set without taking: it is a list to check, not
	// a reading to avoid repeating.
	ranked := rankClaims(in)
	built := []Section{
		{Kind: crmcontracts.MeetingBriefSectionKindHeader, Sentences: headerSection(in)},
		{Kind: crmcontracts.MeetingBriefSectionKindGoal, Sentences: goalSection(in, ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindWhatChanged, Sentences: whatChangedSection(in, ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindAttendees, Sentences: attendeesSection(in)},
		{Kind: crmcontracts.MeetingBriefSectionKindRisks, Sentences: risksSection(in, ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindCommitments, Sentences: briefCopyFor(in.Language).commitmentsSection(ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindDealState, Sentences: dealStateSection(in, ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindTalkingPoints, Sentences: briefCopyFor(in.Language).talkingPointsSection(ranked)},
		{Kind: crmcontracts.MeetingBriefSectionKindCompanyContext, Sentences: companyContextSection(in)},
	}
	// Rendered in the spec's order whatever order they were filled in.
	sort.SliceStable(built, func(i, j int) bool { return specOrder[built[i].Kind] < specOrder[built[j].Kind] })
	for i := range built {
		built[i].Sentences = claims.Dedupe(built[i].Sentences)
	}
	return built
}

// headerSection (D) is what the meeting IS: when, with whom, about which deal,
// and how long since anyone in the room last heard from us.
func headerSection(in Input) []Sentence {
	self := []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}}
	out := []Sentence{{Text: meetingLine(in), Evidence: self}}
	// Which engagement, before which deal: on an account running two bodies of
	// work the project is what tells the reader they are in the right room, and
	// a deal line alone leaves that to be inferred from the deal's name.
	if in.Project != nil {
		out = append(out, Sentence{Text: briefCopyFor(in.Language).projectHeaderLine(*in.Project), Evidence: self})
	}
	if in.Deal != nil {
		out = append(out, Sentence{
			Text:     briefCopyFor(in.Language).dealHeaderLine(*in.Deal),
			Evidence: []Evidence{{EntityType: citeDeal, EntityID: in.Deal.ID}},
		})
	}
	out = append(out, Sentence{Text: lastTouchLine(in), Evidence: self})
	return out
}

// dealHeaderLine states the commercial stake in one line: what is on the table,
// where it sits, and when it is meant to land.
func (said briefCopy) dealHeaderLine(deal DealIn) string {
	parts := []string{deal.Name}
	if amount := contactcontext.SpokenAmount(deal.AmountMinor, deal.Currency); amount != "" {
		parts = append(parts, amount)
	}
	if deal.Stage != "" {
		parts = append(parts, deal.Stage)
	}
	if deal.CloseDate != nil {
		parts = append(parts, fmt.Sprintf(said.Say(headerWords.Close), deal.CloseDate.Format(time.DateOnly)))
	}
	return strings.Join(parts, " · ") + "."
}

// lastTouchLine says how long the room has been quiet. Days, not a timestamp:
// the reader is deciding whether to open with an apology, and "eleven days"
// answers that where a date makes them do the arithmetic.
func lastTouchLine(in Input) string {
	said := briefCopyFor(in.Language)
	if in.LastTouchAt == nil {
		return said.Say(touchWords.None)
	}
	days := elapsed.Days(*in.LastTouchAt, in.Now, in.Zone)
	switch {
	case days <= 0:
		return said.Say(touchWords.Today)
	case days == 1:
		return said.Say(touchWords.Yesterday)
	default:
		return fmt.Sprintf(said.Say(touchWords.Days), days)
	}
}

// goalSection takes the sharpest open ask before the later sections can repeat it.
func goalSection(in Input, ranked *rankedClaims) []Sentence {
	if ask, ok := ranked.take(openOfKind(kindCommitmentOurs, kindOpenQuestion, kindDecision)); ok {
		return []Sentence{{
			Text:     briefCopyFor(in.Language).goalLine(ask, in.Now),
			Nature:   natureRecommendation,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: ask.SourceID}},
		}}
	}
	// A delivery meeting can have no open claim. Cite its project link through
	// the meeting because project is not an admitted evidence type.
	if in.Project != nil {
		return []Sentence{{
			Text:     briefCopyFor(in.Language).projectGoalLine(*in.Project),
			Nature:   natureRecommendation,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}},
		}}
	}
	return nil
}

func (said briefCopy) goalLine(ask ClaimIn, now time.Time) string {
	switch ask.Kind {
	case kindOpenQuestion:
		return fmt.Sprintf(said.Say(sectionWords.AnswerQuestion), ask.ContactName, ask.Body)
	case kindDecision:
		return fmt.Sprintf(said.Say(sectionWords.GetDecision), ask.ContactName, ask.Body)
	default:
		if deadline.Passed(ask.DueAt, now) {
			return fmt.Sprintf(said.Say(sectionWords.CloseOverdue), ask.ContactName, ask.DueAt.UTC().Format(time.DateOnly), ask.Body)
		}
		return fmt.Sprintf(said.Say(sectionWords.ClosePromise), ask.ContactName, ask.Body)
	}
}

// attendeesSection (D list + M one-liners) names the room, with the contacts the
// reader has never spoken to flagged.
//
// The first-time flag is the point of the section. Walking in without knowing
// that a decision-maker in the room has never heard from you is the failure it
// exists to prevent, so it is stated in words rather than left to a badge the
// prose does not mention.
func attendeesSection(in Input) []Sentence {
	out := make([]Sentence, 0, len(in.Attendees))
	for _, attendee := range in.Attendees {
		out = append(out, Sentence{
			Text:     briefCopyFor(in.Language).attendeeLine(attendee, in.Now, in.Zone),
			Evidence: []Evidence{{EntityType: citeContact, EntityID: attendee.ContactID}},
		})
	}
	return out
}

func (said briefCopy) attendeeLine(attendee AttendeeIn, now time.Time, loc *time.Location) string {
	parts := []string{attendee.FullName}
	if attendee.Title != "" {
		parts = append(parts, attendee.Title)
	}
	if attendee.DealRole != "" {
		parts = append(parts, strings.ReplaceAll(dealrole.Word(attendee.DealRole, string(said.Lang())), "_", " "))
	}
	line := strings.Join(parts, ", ")
	// FirstTime and a nil LastTouch are ONE fact with two spellings: the
	// assembler derives the flag from the date (attendeeRow.firstTime), so an
	// attendee with no recorded contact IS the first-time case. Reading only
	// the flag made the pair a dereference waiting for a caller that set
	// neither — and the brief panicked instead of degrading to its floor.
	if attendee.FirstTime || attendee.LastTouch == nil {
		return fmt.Sprintf(said.Say(touchWords.First), line)
	}
	days := elapsed.Days(*attendee.LastTouch, now, loc)
	if days <= 0 {
		return fmt.Sprintf(said.Say(touchWords.SpokeToday), line)
	}
	if days == 1 {
		return fmt.Sprintf(said.Say(touchWords.SpokeYesterday), line)
	}
	return fmt.Sprintf(said.Say(touchWords.SpokeDays), line, days)
}

// commitmentsSection (D) is what is outstanding, ours and theirs, each with the
// conversation it was made in and where it stands.
//
// Ours come first. A rep who walks in without having done what they promised
// has a different meeting than one who has, and reading their own overdue
// promise first is what changes the opening sentence.
// The ledger: every promise and question, complete, in rank order. It does
// not take from the set — the goal and a risk may each name one of these
// lines as a reading of it, and the ledger still has to show the whole of
// what is owed.
func (said briefCopy) commitmentsSection(ranked *rankedClaims) []Sentence {
	claims := ranked.all(ofKind(kindCommitmentOurs, kindCommitmentTheirs, kindOpenQuestion))
	out := make([]Sentence, 0, len(claims))
	for _, claim := range claims {
		out = append(out, Sentence{
			Text:     said.commitmentLine(claim),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		})
	}
	return out
}

func (said briefCopy) commitmentLine(claim ClaimIn) string {
	phrase := headerWords.Asked
	switch claim.Kind {
	case kindCommitmentOurs:
		phrase = headerWords.Ours
		if claim.Status != statusOpen {
			phrase = headerWords.PromisedOurs
		}
	case kindCommitmentTheirs:
		phrase = headerWords.Theirs
		if claim.Status != statusOpen {
			phrase = headerWords.PromisedTheirs
		}
	}
	line := fmt.Sprintf(said.Say(phrase), claim.ContactName, claim.Body)
	if claim.DueAt != nil {
		line += fmt.Sprintf(said.Say(headerWords.Due), claim.DueAt.UTC().Format(time.DateOnly))
	}
	if source := commitmentSource(claim); source != "" {
		line += fmt.Sprintf(said.Say(headerWords.Source), source)
	}
	return fmt.Sprintf(said.Say(headerWords.WithStatus), line, said.claimStatus(claim.Status))
}

// commitmentSource names the conversation in prose. The label is the thread
// subject; without one the sentence says nothing rather than pasting the record
// id, which the grounding filter would drop the whole sentence for.
func commitmentSource(claim ClaimIn) string {
	if claim.SourceLabel == "" {
		return ""
	}
	return fmt.Sprintf("%q", claim.SourceLabel)
}

// dealStateSection (M) is where the deal stands: what was last said, what was
// objected to, and what nobody has decided.
func dealStateSection(in Input, ranked *rankedClaims) []Sentence {
	out := make([]Sentence, 0, dealStateCap)
	if len(in.Recent) > 0 {
		last := in.Recent[0]
		out = append(out, Sentence{
			Text:     briefCopyFor(in.Language).lastConversationLine(last),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: last.ID}},
		})
	}
	// Settled things: the decisions taken. An open objection is a risk and was
	// taken there; open asks went to the goal and the ledger; what they said
	// matters is what they will raise, so it is a talking point.
	for _, claim := range ranked.takeAll(ofKind(kindDecision), dealStateCap-len(out)) {
		out = append(out, Sentence{
			Text:     briefCopyFor(in.Language).dealStateLine(claim),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		})
	}
	return out
}

// dealStateCap is the spec's three-to-five bullets. The ceiling is enforced;
// the floor is not, because padding to three would mean writing a bullet the
// records do not support.
const dealStateCap = 5

func (said briefCopy) lastConversationLine(last ActIn) string {
	subject := said.subjectOrKind(last)
	switch last.Direction {
	case "inbound":
		return fmt.Sprintf(said.Say(sectionWords.TheyWrote), subject)
	case "outbound":
		return fmt.Sprintf(said.Say(sectionWords.WeWrote), subject)
	default:
		return fmt.Sprintf(said.Say(sectionWords.LastCaptured), subject)
	}
}

func (said briefCopy) dealStateLine(claim ClaimIn) string {
	return fmt.Sprintf(said.Say(sectionWords.Agreed), claim.ContactName, claim.Body)
}

// risksSection (M, ≤3) is OMITTED when empty, and that is spelled in the spec
// rather than inferred. A risks heading over nothing reads as "we looked and
// found none", which is a claim this floor cannot make.
func risksSection(in Input, ranked *rankedClaims) []Sentence {
	isRisk := func(c ClaimIn) bool { _, ok := briefCopyFor(in.Language).riskLine(c, in.Now); return ok }
	claims := ranked.takeAll(isRisk, riskCap)
	out := make([]Sentence, 0, len(claims))
	for _, claim := range claims {
		text, _ := briefCopyFor(in.Language).riskLine(claim, in.Now)
		out = append(out, Sentence{
			Text:     text,
			Nature:   natureAssessment,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		})
	}
	return out
}

const riskCap = 3

// riskLine turns a claim into a watch-out only when the RECORD says something
// is wrong: a promise of ours past its date, or an objection nobody closed. A
// risk read out of anything else would be the deal-history-ignoring filler the
// spec forbids.
func (said briefCopy) riskLine(claim ClaimIn, now time.Time) (string, bool) {
	if claim.Kind == kindObjection && claim.Status == statusOpen {
		return fmt.Sprintf(said.Say(sectionWords.OpenObjection), claim.ContactName, claim.Body), true
	}
	overdue := claim.Kind == kindCommitmentOurs &&
		claim.Status == statusOpen &&
		deadline.Passed(claim.DueAt, now)
	if overdue {
		return fmt.Sprintf(said.Say(sectionWords.OweOverdue), claim.ContactName, claim.Body), true
	}
	return "", false
}

// talkingPointsSection (M, ≤5) is each point tied to a specific captured
// statement — never a generic opener, because a talking point nobody said is
// the filler the first hard rule forbids. Each point is a MOVE: the evidence
// and what to do with it in the room, not a label on a record.
func (said briefCopy) talkingPointsSection(ranked *rankedClaims) []Sentence {
	candidate := ofKind(kindObjection, kindDecisionProcess, kindPriority, kindSuccessCriterion, kindCommitmentTheirs)
	claims := ranked.takeAll(func(c ClaimIn) bool { return candidate(c) && said.talkingPointLine(c) != "" }, talkingPointCap)
	out := make([]Sentence, 0, len(claims))
	for _, claim := range claims {
		out = append(out, Sentence{
			Text:     said.talkingPointLine(claim),
			Nature:   natureRecommendation,
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: claim.SourceID}},
		})
	}
	return out
}

const talkingPointCap = 5

func (said briefCopy) talkingPointLine(claim ClaimIn) string {
	if claim.Kind == kindCommitmentTheirs && claim.Status != statusOpen {
		return ""
	}
	switch claim.Kind {
	case kindObjection:
		if claim.Status == statusOpen {
			return fmt.Sprintf(said.Say(sectionWords.AnswerObjection), claim.ContactName, claim.Body)
		}
		return fmt.Sprintf(said.Say(sectionWords.CheckObjection), claim.ContactName, claim.Body)
	case kindDecisionProcess:
		return fmt.Sprintf(said.Say(sectionWords.DecisionProcess), claim.ContactName, claim.Body)
	case kindSuccessCriterion:
		return fmt.Sprintf(said.Say(sectionWords.Success), claim.ContactName, claim.Body)
	case kindCommitmentTheirs:
		return fmt.Sprintf(said.Say(sectionWords.TheirPromise), claim.ContactName, claim.Body)
	default:
		return fmt.Sprintf(said.Say(sectionWords.Priority), claim.ContactName, claim.Body)
	}
}

// companyContextSection recalls earlier meetings with this room; no history
// means no background section.
func companyContextSection(in Input) []Sentence {
	out := make([]Sentence, 0, len(in.PriorMeetings))
	for _, prior := range in.PriorMeetings {
		out = append(out, Sentence{
			Text:     briefCopyFor(in.Language).priorMeetingLine(prior, in.Now, in.Zone),
			Evidence: []Evidence{{EntityType: citeActivity, EntityID: prior.ID}},
		})
	}
	return out
}
