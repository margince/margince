// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"fmt"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var doneWords = struct{ YourEmail, MaybeDone, Done langcopy.Phrase }{
	YourEmail: langcopy.Phrase{En: "Your email", De: "Ihre E-Mail", Vi: "Email của bạn"},
	MaybeDone: langcopy.Phrase{En: "You may have done this — you wrote to them on %s", De: "Möglicherweise schon erledigt: Sie haben am %s geschrieben", Vi: "Bạn có thể đã hoàn thành việc này: bạn đã viết cho họ vào ngày %s"},
	Done:      langcopy.Phrase{En: "Done", De: "Erledigt", Vi: "Hoàn thành"},
}

// DoneQuestion is a promise card and the later mail that may have kept it.
type DoneQuestion struct {
	Plain     crmcontracts.ContactMoment
	Kind      crmcontracts.ContactMomentMayBeDonePromiseType
	PromiseID openapi_types.UUID
	Sent      activities.PromiseFollowUp
	// Key names the question apart from the plain card, so Not yet puts away
	// the question and the plain card comes back.
	Key string
}

// DoneEvidence is the plain card's own evidence plus the mail. The mail is in
// the fingerprint, so Not yet holds only until we write on the thread again.
func DoneEvidence(said langcopy.Spoken, q DoneQuestion) []crmcontracts.ContactMomentEvidence {
	label := q.Sent.Subject
	if label == "" {
		label = said.Say(doneWords.YourEmail)
	}
	at, email := q.Sent.At, openapi_types.UUID(q.Sent.EmailID)
	return []crmcontracts.ContactMomentEvidence{q.Plain.Evidence[0], {
		Type: crmcontracts.ContactMomentEvidenceTypeActivity, Id: &email, Label: label, ObservedAt: &at,
		ObservedPrecision: new(crmcontracts.OccurredPrecision(q.Sent.Precision)),
	}}
}

// AskDone is the card that asks whether the mail kept the promise. Both
// record pages draw it, so both say the same sentence and offer one Done.
func AskDone(said langcopy.Spoken, zone *time.Location, q DoneQuestion, evidence []crmcontracts.ContactMomentEvidence, fingerprint string) crmcontracts.ContactMoment {
	at := q.Sent.At
	return crmcontracts.ContactMoment{
		ClaimKey:            q.Key,
		Rule:                q.Plain.Rule,
		RuleVersion:         q.Plain.RuleVersion,
		EvidenceFingerprint: fingerprint,
		Headline:            fmt.Sprintf(said.Say(doneWords.MaybeDone), at.In(zone).Format(time.DateOnly)),
		WhyNow:              strings.TrimRight(q.Plain.Headline, ".") + ". " + q.Plain.WhyNow,
		// Inferred from a reply on the thread, not observed: the mail may be
		// about something else.
		Confidence:  crmcontracts.ContactMomentConfidenceMedium,
		Evidence:    evidence,
		FreshnessAt: &at,
		RecommendedAction: crmcontracts.ContactMomentAction{
			Kind:  crmcontracts.ContactMomentActionKindCompleteTask,
			Label: said.Say(doneWords.Done),
			State: crmcontracts.ContactMomentActionStateAvailable,
		},
		MayBeDone: &crmcontracts.ContactMomentMayBeDone{
			PromiseType: q.Kind, PromiseId: q.PromiseID, EmailActivityId: *evidence[1].Id, WroteAt: at,
		},
	}
}
