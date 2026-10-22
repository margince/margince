// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a commitment's approval email says beyond its subject. It names the
// record, the source, the promiser, the promise and why the reader is asked.
// Every fact is read as the recipient through the card preview's readers, so
// the email names nothing the card would not show them.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalMailFacts is the single email's view of one approval. Rich is false
// when the approval has no facts to name, and then the email reads as it always
// has.
type approvalMailFacts struct {
	Rich     bool
	Heading  string
	Lines    []approvalMailLine
	Deadline string
}

// approvalMailLine is one labelled fact.
type approvalMailLine struct {
	Label, Value string
}

// commitmentMailFacts builds the facts for a commitment, read as the
// recipient. A fact they may not see is left out. The quote is the one the
// preview built for them, so it passed the check the card's citation did.
func (r approvalPreviewReader) commitmentMailFacts(
	ctx context.Context, approval crmcontracts.Approval, words mailcopy.Copy, local func(time.Time) time.Time,
) (approvalMailFacts, error) {
	facts := approvalMailFacts{Rich: true}
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return facts, err
	}
	var proposal CommitmentTaskProposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		return facts, nil //nolint:nilerr // A malformed payload names nothing; the email keeps its subject and deadline.
	}
	field, id := commitmentRecord(proposal.Links)
	record, err := r.optionalLabel(ctx, field, id)
	if err != nil {
		return facts, err
	}
	facts.Heading = record
	facts.add(words.MailAbout, record)
	source, err := r.commitmentSourceLine(ctx, proposal.SourceActivityID, words, local)
	if err != nil {
		return facts, err
	}
	facts.add(words.MailReadFrom, source)
	var promiser string
	if proposal.PromiserSeatID != nil {
		if promiser, err = r.optionalLabel(ctx, bulkParamOwner, proposal.PromiserSeatID.String()); err != nil {
			return facts, err
		}
	}
	facts.add(words.MailPromisedBy, promiser)
	facts.add(words.MailInTheirWords, previewQuote(approval.Preview, proposal.SourceActivityID))
	if approval.Preview != nil {
		facts.add(words.MailWhyYou, reviewerSentence(approval.Preview.ReviewerReason, words))
	}
	return facts, nil
}

func (f *approvalMailFacts) add(label, value string) {
	if value != "" {
		f.Lines = append(f.Lines, approvalMailLine{Label: label, Value: value})
	}
}

// commitmentRecord is the deal the promise's task lands on, else its company,
// as a reference field and id. A contact is never named: their name in a
// mailbox is personal data the email does not need.
func commitmentRecord(links []activities.ActivityLinkInput) (string, string) {
	for _, kind := range []string{string(recordTypeDeal), string(recordTypeCompany)} {
		for _, link := range links {
			if link.EntityType == kind {
				return kind + "_id", link.EntityID.String()
			}
		}
	}
	return "", ""
}

// optionalLabel names one record as the recipient may see it, or nothing when
// there is no record or they may not see it.
func (r approvalPreviewReader) optionalLabel(ctx context.Context, field, id string) (string, error) {
	if field == "" {
		return "", nil
	}
	value, err := r.referenceValue(ctx, field, id)
	if hiddenFromRecipient(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value.Text, nil
}

// commitmentSourceLine names the message the promise was read from: an
// email's subject and date, or a meeting transcript's date.
func (r approvalPreviewReader) commitmentSourceLine(
	ctx context.Context, source ids.UUID, words mailcopy.Copy, local func(time.Time) time.Time,
) (string, error) {
	activity, err := r.activities.GetActivityContent(ctx, ids.From[ids.ActivityKind](source), storekit.LiveOnly)
	if hiddenFromRecipient(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	day := words.Date(local(activity.OccurredAt))
	if activity.Kind != crmcontracts.ActivityKindEmail {
		return words.MailTranscriptOf + " " + day, nil
	}
	if activity.Subject == nil || *activity.Subject == "" {
		return day, nil
	}
	return *activity.Subject + ", " + day, nil
}

func hiddenFromRecipient(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied)
}

// previewQuote is the promise sentence the preview cited for this recipient.
// It is empty unless the preview is ready and cites the proposal's own source.
// Reading the source again would be a second answer to whether they may see it.
func previewQuote(preview *crmcontracts.ApprovalPreview, source ids.UUID) string {
	if preview == nil || preview.State != previewReadyState || preview.Evidence == nil {
		return ""
	}
	for _, cited := range *preview.Evidence {
		if cited.SourceId != nil && ids.UUID(*cited.SourceId) == source {
			return cited.EvidenceSnippet
		}
	}
	return ""
}

// reviewerSentence is the card's sentence for why the reader is asked.
func reviewerSentence(reason string, words mailcopy.Copy) string {
	switch reason {
	case reviewerRequestedDecider:
		return words.MailReasonRequested
	case reviewerOnBehalfOf:
		return words.MailReasonOnBehalf
	case reviewerRecordAuthority:
		return words.MailReasonAuthority
	}
	return ""
}
