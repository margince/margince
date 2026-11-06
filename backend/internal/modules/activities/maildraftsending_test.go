// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

var draftSavedAt = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)

func TestADraftHoldsItsReasonAndMomentToTheSendsRules(t *testing.T) {
	t.Parallel()
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeContact, ID: ids.NewV7()}
	tooMany := make([]ids.UUID, 0, maxAttachmentsPerSend+1)
	for range maxAttachmentsPerSend + 1 {
		tooMany = append(tooMany, ids.NewV7())
	}
	inTwoDays := &SendSchedule{At: draftSavedAt.Add(48 * time.Hour), TZ: "Europe/Berlin"}
	for name, tc := range map[string]struct {
		content MailDraftContent
		refusal error
	}{
		"a claimable reason":       {MailDraftContent{Context: commsauthz.CategoryMarketing}, nil},
		"a reserved reason":        {MailDraftContent{Context: commsauthz.CategorySecurityNotice}, &CommunicationContextError{}},
		"an unknown reason":        {MailDraftContent{Context: "small_talk"}, &CommunicationContextError{}},
		"a moment in two days":     {MailDraftContent{Schedule: inTwoDays}, nil},
		"a moment already passed":  {MailDraftContent{Schedule: &SendSchedule{At: draftSavedAt.Add(-time.Hour), TZ: "UTC"}}, nil},
		"a moment past the limit":  {MailDraftContent{Schedule: &SendSchedule{At: draftSavedAt.Add(scheduleCeiling + time.Hour), TZ: "UTC"}}, &InvalidScheduleError{}},
		"a numeric offset as zone": {MailDraftContent{Schedule: &SendSchedule{At: inTwoDays.At, TZ: "+02:00"}}, &InvalidScheduleError{}},
		"more files than a send":   {MailDraftContent{AttachmentIDs: tooMany}, &TooManyAttachmentsError{}},
	} {
		err := validateDraft(anchor, tc.content, draftSavedAt)
		switch want := tc.refusal.(type) {
		case nil:
			if err != nil {
				t.Errorf("%s: refused with %v", name, err)
			}
		case *CommunicationContextError:
			if !errors.As(err, &want) {
				t.Errorf("%s: %v, want the send's reason refusal", name, err)
			}
		case *InvalidScheduleError:
			if !errors.As(err, &want) {
				t.Errorf("%s: %v, want the send's schedule refusal", name, err)
			}
		case *TooManyAttachmentsError:
			if !errors.As(err, &want) {
				t.Errorf("%s: %v, want the send's file cap", name, err)
			}
		}
	}
}

func TestADraftSaveRefusesAMomentWithoutItsZone(t *testing.T) {
	t.Parallel()
	at := draftSavedAt
	_, err := mailDraftContentFrom(crmcontracts.MailDraftInput{ScheduledAt: &at})
	var refused *InvalidScheduleError
	if !errors.As(err, &refused) || refused.Field != FieldScheduledTZ {
		t.Fatalf("a moment with no zone → %v, want a refusal naming %s", err, FieldScheduledTZ)
	}
}

func TestADraftSaveOmittingTheSendingFieldsMeansNone(t *testing.T) {
	t.Parallel()
	content, err := mailDraftContentFrom(crmcontracts.MailDraftInput{})
	if err != nil {
		t.Fatalf("an old client's body → %v", err)
	}
	if len(content.AttachmentIDs) != 0 || content.Context != "" || content.Schedule != nil {
		t.Errorf("an old client's body read as %+v, want no files, no reason and no moment", content)
	}
}

func TestADraftServesItsFilesReasonAndMoment(t *testing.T) {
	t.Parallel()
	file := OutboundFile{AttachmentID: ids.NewV7(), Filename: "quote.pdf", ContentType: "application/pdf", ByteSize: 2048, Checksum: "kept-off-the-wire"}
	out := mailDraftResponse(MailDraft{
		Content: MailDraftContent{
			Context:  commsauthz.CategoryPrecontractQuote,
			Schedule: &SendSchedule{At: draftSavedAt, TZ: "Europe/Berlin"},
		},
		Files: resolvedFiles{attachable: []OutboundFile{file}, unavailable: 2},
	})
	if out.Attachments == nil || len(*out.Attachments) != 1 {
		t.Fatalf("served attachments %v, want the one the reader may attach", out.Attachments)
	}
	want := crmcontracts.ApprovalPreviewFile{
		Id: openapi_types.UUID(file.AttachmentID), Filename: "quote.pdf", ContentType: "application/pdf", ByteSize: 2048,
	}
	if (*out.Attachments)[0] != want {
		t.Errorf("served %+v, want %+v so the composer draws the chip without asking again", (*out.Attachments)[0], want)
	}
	if out.AttachmentsUnavailable == nil || *out.AttachmentsUnavailable != 2 {
		t.Errorf("served %v unavailable, want 2", out.AttachmentsUnavailable)
	}
	if out.CommunicationContext == nil || *out.CommunicationContext != crmcontracts.CommunicationContextPrecontractQuote {
		t.Errorf("served reason %v, want precontract_quote", out.CommunicationContext)
	}
	if out.ScheduledAt == nil || !out.ScheduledAt.Equal(draftSavedAt) || out.ScheduledTz == nil || *out.ScheduledTz != "Europe/Berlin" {
		t.Errorf("served moment %v in %v, want %v in Europe/Berlin", out.ScheduledAt, out.ScheduledTz, draftSavedAt)
	}
}

func TestADraftWithNothingChosenServesNoSendingFields(t *testing.T) {
	t.Parallel()
	out := mailDraftResponse(MailDraft{})
	if out.Attachments != nil || out.AttachmentsUnavailable != nil || out.CommunicationContext != nil ||
		out.ScheduledAt != nil || out.ScheduledTz != nil {
		t.Errorf("an empty draft served %+v, want every sending field absent", out)
	}
}
