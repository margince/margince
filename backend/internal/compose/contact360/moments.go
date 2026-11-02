// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Why this contact needs attention TODAY — one answer, not a list.
//
// The page's opening line is a reason, not a record. "Warm, 73" describes; "she
// replied after 41 quiet days and nobody has answered" is something to do.
//
// ONE MOMENT WINS (ADR-0096 D2). A card offering five reasons has handed the
// choosing back to the reader, which is the work this ladder exists to do. The
// ladder is fixed and ordered by consequence; the first rung that fires is the
// answer, and the rest are not computed into a list nobody reads.
//
// Every moment here is DETERMINISTIC — derived by a rule from captured
// activity, never asserted by a model. Three things follow from that, and all
// three are the point:
//
//	Every moment carries evidence the reader can open. A rule knows what it
//	fired on; a paraphrase does not.
//
//	The page opens on a reason at FIRST PAINT. Nothing here waits on a model,
//	so there is no placeholder state pretending to be an answer.
//
//	A moment cannot be wrong in the way a generated one can. It can be
//	unwelcome — which is what dismissal is for — but it cannot assert
//	something that did not happen.
//
// They are derived from the sections this page has ALREADY read rather than
// from fresh queries. That costs nothing extra and buys an invariant: a moment
// can never cite evidence the page beside it is not showing, and a section the
// caller may not read contributes no moments rather than leaking through one.
//
// Two rungs are input-bound and stay dormant rather than being mocked into
// life: job_change fires only on a RECORDED employment change, and
// public_signal needs a connected data provider. A rule that cannot fire is
// absent from the page, not an empty card.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ruleVersion stamps the ladder that selected a moment. It changes whenever a
// rung's condition or order changes, so the same evidence rendering differently
// across two clients is visible rather than silent.
const ruleVersion = "contact-moment-ladder-v6"

// meetingHorizonHours is how far ahead a meeting is worth preparing for
// (ADR-0096 D2 rung 1). Three days, not the week the earlier ladder used: a
// meeting on Friday is not what a reader should be told to do about on Monday,
// and the prep moment is worth something only while there is still time to act
// on it.
const meetingHorizonHours = 72

// reEngagedQuietDays is the silence a new inbound has to break before its
// arrival is a moment rather than ordinary correspondence.
const reEngagedQuietDays = 14

// goneQuietAfterDays is how long our outbound may go unanswered before the
// silence is the point. It is the configured rule the moment names in its own
// text, so the reader can see what verdict they are being shown.
const goneQuietAfterDays = 7

// prefillIntent is the prefill key a composer reads to know what it is opening
// for. One spelling, because a typo here is a silently empty drawer.
const prefillIntent = "intent"

// deliverCommitmentIntent opens the composer to keep a promise the contact was made.
const deliverCommitmentIntent = "deliver_commitment"

// prefillSubject is the prefill key for the composer's subject line.
const prefillSubject = "subject"

// momentsSection selects the one moment this page opens on, and honours a
// dismissal the viewer has already made against the same evidence.
//
// It runs LAST among the sections so it can read what the others gathered.
func (s *Service) momentsSection(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time, opts AssembleOptions, out *crmcontracts.Contact360) error {
	putAway, err := s.momentDismissals(ctx, tx, contactID)
	if err != nil {
		return err
	}
	dismissed := func(moment crmcontracts.ContactMoment) bool {
		stored, ok := putAway[moment.ClaimKey]
		return ok && stored == moment.EvidenceFingerprint
	}
	zone, err := recordZone(ctx, tx)
	if err != nil {
		return err
	}
	said := momentCopyFor(string(identity.ReaderLanguageForRecord(ctx, tx)))
	said.zone = zone
	// A reader who may not read activity sees no mail to cite, so their
	// promise cards stay as they are.
	followUps, err := promiseFollowUps(ctx, tx, contactID, now, out)
	if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return err
	}
	moment := said.deriveMomentPast(ctx, now, out, dismissed, said.proposer(ctx, out, followUps, dismissed))
	momentaction.Withhold(ctx, string(said.Lang()), &moment)
	out.Moment = &moment
	return nil
}

// momentDismissals reads the moments this viewer has put away on this contact,
// as the fingerprint each was showing, keyed by claim key.
//
// A moment counts as dismissed only while its fingerprint still matches. A
// dismissal keyed on the moment's path alone survives the world changing
// underneath it: the reader dismisses "she went quiet", a reply arrives, and
// the page stays silent about the thing that just changed. Keyed on the
// evidence, the dismissal re-arms.
func (s *Service) momentDismissals(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (map[string]string, error) {
	// A dismissal belongs to a contact's screen, so a call carrying no user has
	// none to honour. An agent reading through a passport must not consume the
	// granting human's: it sees every moment. This is a fact about the caller,
	// not a failure to read.
	//
	// The agent arm is the one that needs saying, because a passport carries
	// its granting human's UserID: a check for a non-zero id admits an agent
	// and reads the human's dismissals, which is what this paragraph promises
	// it does not do. auth.RequireHuman is what tells the two apart.
	viewer, ok := principal.Actor(ctx)
	if !ok || viewer.UserID == (ids.UUID{}) {
		return map[string]string{}, nil
	}
	if err := auth.RequireHuman(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT claim_key, evidence_fingerprint
		FROM contact_moment_dismissal
		WHERE user_id = $1 AND contact_id = $2`,
		viewer.UserID, contactID)
	if err != nil {
		return nil, fmt.Errorf("read moment dismissals: %w", err)
	}
	defer rows.Close()
	putAway := map[string]string{}
	for rows.Next() {
		var key, fingerprint string
		if err := rows.Scan(&key, &fingerprint); err != nil {
			return nil, fmt.Errorf("read moment dismissals: %w", err)
		}
		putAway[key] = fingerprint
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read moment dismissals: %w", err)
	}
	return putAway, nil
}

// deriveMoment walks the ladder in ADR-0096's fixed order and returns the first
// rung that fires.
//
// The order is the decision, not a score: a meeting in two days outranks a
// silence of three weeks because one has a deadline and the other does not. A
// number here would imply a precision the rules do not have.
//
// Rungs 3 (job change) and 7 (public signal) are absent by design — both need
// inputs this build does not have, and a rule that cannot fire belongs nowhere
// on the page.
func (said momentCopy) deriveMoment(ctx context.Context, now time.Time, page *crmcontracts.Contact360) crmcontracts.ContactMoment {
	return said.deriveMomentPast(ctx, now, page, func(crmcontracts.ContactMoment) bool { return false }, asAssembled)
}

// deriveMomentPast walks the same ladder and skips the rungs whose card this
// reader has already put away.
//
// A DISMISSAL SILENCES ONE CARD, NOT THE RECORD. Stopping at the first rung
// and answering "nothing needs you today" when that card was dismissed says
// something false about everything below it: a reader who puts one promise
// away is told the contact needs nothing while a second promise sits open.
// The card they dismissed is the card they meant, and the next reason down is
// still a reason.
//
// A rung that fires and is dismissed is passed over rather than ending the
// walk, so the ladder's order still decides what the reader sees — they see
// the highest rung they have NOT put away.
func (said momentCopy) deriveMomentPast(
	ctx context.Context, now time.Time, page *crmcontracts.Contact360,
	dismissed func(crmcontracts.ContactMoment) bool,
	propose func(crmcontracts.ContactMoment) crmcontracts.ContactMoment,
) crmcontracts.ContactMoment {
	if notice, ok := said.noticeDutyMoment(now, page); ok {
		return notice
	}
	for _, plain := range said.ladder() {
		rung := proposing(plain, propose)
		moment, ok := rung(ctx, now, page)
		if !ok || !dismissed(moment) {
			if ok {
				return moment
			}
			continue
		}
		// The rung fired and this reader has put THAT card away. Ask it again
		// without the dismissed card in front of it: a rung speaks for a set —
		// three open promises, one card — so passing over the rung would hide
		// the other two along with the one that was dismissed.
		if next, ok := rungPast(ctx, now, page, rung, dismissed); ok {
			return next
		}
	}
	// 10. Nothing needs you today. A quiet success state, not a blank card:
	// "there is nothing here" is an answer, and the reader came for an answer.
	return said.nothingNeededMoment(ctx, now, page)
}

// ladderRung answers one rung's question over the assembled page.
type ladderRung = func(context.Context, time.Time, *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool)

// A meeting deadline wins over a reply, followed by promises and relationship gaps.
// Role changes need recorded role-change evidence; reply gaps cannot establish them.
func (said momentCopy) ladder() []ladderRung {
	return []ladderRung{said.meetingPrepMoment, said.reEngagedMoment, said.overduePromiseMoment, said.goneQuietMoment, said.openPromiseMoment, said.missingNextStepMoment, said.thinRelationshipMoment}
}

// momentLadderNames names the rungs in ladder order, for tests that
// report which rung they are talking about. A rung that does not fire returns
// a zero ContactMoment whose Rule is empty, so the name cannot come from the
// value — it has to be stated here.
var momentLadderNames = []string{
	"meeting_prep",
	"re_engaged",
	"overdue_promise",
	"gone_quiet",
	"open_promise",
	"missing_next_step",
	"thin_relationship",
}

// meetingPrepMoment: a meeting is close enough that preparing for it is the
// most valuable thing the reader could do.
func (said momentCopy) meetingPrepMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.NextMeeting == nil {
		return crmcontracts.ContactMoment{}, false
	}
	meeting := *page.NextMeeting
	if !meeting.StartsAt.After(now) || meeting.StartsAt.After(now.Add(meetingHorizonHours*time.Hour)) {
		return crmcontracts.ContactMoment{}, false
	}
	label := said.Say(momentWords.Meeting)
	if meeting.Subject != nil && *meeting.Subject != "" {
		label = *meeting.Subject
	}
	id := meeting.ActivityId
	evidence := []crmcontracts.ContactMomentEvidence{{
		Type:              crmcontracts.ContactMomentEvidenceTypeActivity,
		Id:                &id,
		Label:             label,
		ObservedAt:        &meeting.StartsAt,
		ObservedPrecision: &meeting.OccurredPrecision,
	}}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:meeting_prep",
		Rule:                crmcontracts.ContactMomentRuleMeetingPrep,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf(said.Say(momentWords.Prepare), label),
		WhyNow:              said.Say(momentWords.PrepareWhy),
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            evidence,
		FreshnessAt:         &meeting.StartsAt,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindOpenMeetingBrief,
			Label: said.Say(momentWords.Brief),
			State: crmcontracts.ContactMomentActionStateAvailable,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface:    crmcontracts.ContactMomentDestinationSurfaceMeetingBrief,
				EntityType: new(crmcontracts.ContactMomentDestinationEntityTypeActivity),
				EntityId:   &id,
			},
		},
		SecondaryActions: &[]crmcontracts.ContactMomentAction{{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.Agenda),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: "agenda"}),
			},
		}},
	}, true
}

// reEngagedMoment: they came back. The strongest reason captured data alone can
// produce, and the one most likely to be acted on the same hour.
func (said momentCopy) reEngagedMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	// Imported mail is history, not a reply, so this rung reads the inbound
	// that is not import-written.
	if page.LastLiveInboundAt == nil {
		return crmcontracts.ContactMoment{}, false
	}
	inbound := *page.LastLiveInboundAt
	if page.LastOutboundAt != nil && !page.LastOutboundAt.Before(inbound) {
		// We answered after they wrote. Nothing is owed, and their message is
		// not news.
		return crmcontracts.ContactMoment{}, false
	}
	// The gap this rung fires on, and it is OURS: the page carries the latest
	// message in each direction and nothing between them, so the only interval
	// derivable here is from our most recent outbound to their most recent
	// inbound. What that measures is how long it has been since WE wrote.
	if page.LastOutboundAt == nil {
		return crmcontracts.ContactMoment{}, false
	}
	sinceWeWrote := occurredday.DaysBetween(
		occurredday.Moment{At: *page.LastOutboundAt, DateOnly: occurredday.IsStatedDay(page.LastOutboundPrecision)},
		occurredday.Moment{At: inbound, DateOnly: occurredday.IsStatedDay(page.LastLiveInboundPrecision)}, said.zone)
	if sinceWeWrote < reEngagedQuietDays {
		return crmcontracts.ContactMoment{}, false
	}
	evidence := []crmcontracts.ContactMomentEvidence{said.inboundEvidence(page, inbound, page.LastLiveInboundPrecision)}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:re_engaged",
		Rule:                crmcontracts.ContactMomentRuleReEngaged,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		// What the two dates support, and no more. "They replied after N quiet
		// days" said the SILENCE was theirs, and the page cannot know that: it
		// holds one timestamp per direction, so every message between the two
		// is invisible here. A contact who wrote every week for two months
		// while nobody answered produced "They replied after 60 quiet days" —
		// a number specific enough to sound checked, about a quiet period that
		// never happened. A rep who tests one of these against the thread and
		// finds it wrong stops believing the parts that are right.
		//
		// The interval itself is real; only its owner was wrong. It is the age
		// of OUR last message, which is the fact this rung fires on anyway.
		Headline:    fmt.Sprintf(said.Say(momentWords.Replied), sinceWeWrote),
		WhyNow:      said.Say(momentWords.RepliedWhy),
		Confidence:  crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:    evidence,
		FreshnessAt: &inbound,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.Reply),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: "reply"}),
			},
		},
	}, true
}

// goneQuietMoment: our outbound has gone unanswered past the configured rule.
//
// The moment names the rule in its own text. A reader who disagrees with the
// verdict can see what produced it, which is the difference between a system
// that judges and one that explains.
func (said momentCopy) goneQuietMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
	if page.LastOutboundAt == nil {
		return crmcontracts.ContactMoment{}, false
	}
	outbound := *page.LastOutboundAt
	if page.LastInboundAt != nil && !page.LastInboundAt.Before(outbound) {
		// They answered. Silence is not the story.
		return crmcontracts.ContactMoment{}, false
	}
	waiting := occurredday.DaysSince(occurredday.Moment{At: outbound, DateOnly: occurredday.IsStatedDay(page.LastOutboundPrecision)}, now, said.zone)
	if waiting < goneQuietAfterDays {
		return crmcontracts.ContactMoment{}, false
	}
	quietFor := waiting
	if page.LastInboundAt != nil {
		quietFor = occurredday.DaysSince(occurredday.Moment{At: *page.LastInboundAt, DateOnly: occurredday.IsStatedDay(page.LastInboundPrecision)}, now, said.zone)
	}
	evidence := said.outboundEvidence(page, outbound, page.LastOutboundPrecision)
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:gone_quiet",
		Rule:                crmcontracts.ContactMomentRuleGoneQuiet,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: fingerprintOf(evidence),
		Headline:            fmt.Sprintf(said.Say(momentWords.Quiet), quietFor),
		WhyNow: fmt.Sprintf(said.Say(momentWords.QuietWhy),
			waiting, goneQuietAfterDays),
		Confidence:  crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:    evidence,
		FreshnessAt: &outbound,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindDraftReply,
			Label: said.Say(momentWords.FollowUp),
			State: crmcontracts.ContactMomentActionStateWillConfirm,
			Destination: &crmcontracts.ContactMomentDestination{
				Surface: crmcontracts.ContactMomentDestinationSurfaceComposer,
				Prefill: new(map[string]string{prefillIntent: "follow_up"}),
			},
		},
		SecondaryActions: &[]crmcontracts.ContactMomentAction{said.askColleague()},
	}, true
}

// askColleague offers the second play on a quiet relationship: somebody else
// here may know why it went quiet.
//
// It is BLOCKED, and that is the honest state rather than a placeholder. None
// of the five destinations reaches a screen for asking a colleague, so an
// available action would render as an enabled button that does nothing when
// pressed — which is what shipped, and what a rep learns to distrust the page
// for.
//
// Blocked keeps the play visible and says why, which is the difference between
// a feature that is coming and one that is broken. Note what is and is not
// missing: the rail's "who knows them" card already NAMES the colleagues, from
// the same GET /contacts/{id}/network read. What does not exist is the step after
// that — a screen for asking one of them — so the reason says that, and not
// that Margince cannot tell who they are. It sits beside a card listing them.
func (said momentCopy) askColleague() crmcontracts.ContactMomentAction {
	reason := said.Say(momentWords.AskUnavailable)
	return crmcontracts.ContactMomentAction{
		Kind:          crmcontracts.ContactMomentActionKindAskColleague,
		Label:         said.Say(momentWords.Ask),
		State:         crmcontracts.ContactMomentActionStateBlocked,
		BlockedReason: &reason,
	}
}

// logInteraction offers the one thing worth doing on a record with nothing
// pending: write down something that happened off-system. It opens the
// log-activity form the contact page mounts, the same form the deal and lead
// pages keep in their rail.
func (said momentCopy) logInteraction() crmcontracts.ContactMomentAction {
	return crmcontracts.ContactMomentAction{
		Kind:  crmcontracts.ContactMomentActionKindLogActivity,
		Label: said.Say(momentWords.Log),
		State: crmcontracts.ContactMomentActionStateAvailable,
		Destination: &crmcontracts.ContactMomentDestination{
			Surface: crmcontracts.ContactMomentDestinationSurfaceActivityLog,
		},
	}
}

// nothingNeededMoment is the quiet success state — rung 10, and the answer far
// more often than any of the others.
//
// It is a moment rather than an absence because the reader opened the page to
// be told what to do, and "nothing" is a legitimate answer that an empty card
// fails to give.
func (said momentCopy) nothingNeededMoment(_ context.Context, now time.Time, page *crmcontracts.Contact360) crmcontracts.ContactMoment {
	why := said.Say(momentWords.QuietNothing)
	// Every rung above this one either fired or found nothing, and "found
	// nothing" includes "was not allowed to look". Saying nobody is waiting on
	// a reply when the timeline was withheld states a fact about data this
	// reader could not see, so the sentence says what is actually true instead.
	if withheld(page, crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedActivities,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedLastTouch,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextMeeting,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextSteps,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedClaims,
		crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedCommercial) {
		why = said.Say(momentWords.HiddenNothing)
	}
	return crmcontracts.ContactMoment{
		ClaimKey:            "moment:nothing_needed",
		Rule:                crmcontracts.ContactMomentRuleNothingNeeded,
		RuleVersion:         new(ruleVersion),
		EvidenceFingerprint: "quiet",
		Headline:            said.Say(momentWords.Nothing),
		WhyNow:              why,
		Confidence:          crmcontracts.ContactMomentConfidenceObservedFact,
		Evidence:            []crmcontracts.ContactMomentEvidence{},
		FreshnessAt:         &now,
		RecommendedAction:   said.logInteraction(),
	}
}

func recordZone(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	return identity.ZoneAppliedTx(ctx, tx)
}
