// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Filing a chat screenshot on the contact it was pasted on. One message
// activity is linked to the contact, and the stored image moves onto it.
// Nothing is written before the user confirms. The reading of the image never
// writes anything.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ChatCaptureSourceSystem keys a filed chat on the reading it was read by. A
// second confirm of the same reading returns the activity the first one wrote.
const ChatCaptureSourceSystem = "chat_screenshot"

// The fields a chat reading reports, in the order it is asked for them. The
// reading in compose asks for them; the confirm here reads them back.
const (
	ChatFieldText  = "chat_text"
	ChatFieldName  = "counterparty_name"
	ChatFieldPhone = "counterparty_phone"
)

// MaxChatText bounds the text of one chat, read or typed, in characters. A
// phone screen shows a few dozen short messages; far more is not a screenshot.
const MaxChatText = 6000

// ChatImageMIMEs are the image types a chat is read from: the photo types the
// image policy reads (ai.CarriesImage), which compose holds equal to this list.
func ChatImageMIMEs() []string { return []string{"image/jpeg", "image/png", "image/gif", "image/webp"} }

// ChatCaptureError is one refused confirm. Field names the request field the
// caller fixes; it is empty when nothing in the request can fix it.
type ChatCaptureError struct{ Field, Code, Message string }

func (e *ChatCaptureError) Error() string { return e.Message }

// FieldFault answers 422 naming the field, when there is one.
func (e *ChatCaptureError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

// ChatCaptureFiling is what a confirm files.
type ChatCaptureFiling struct {
	AttachmentID ids.UUID
	ReadingID    ids.UUID
	Channel      string
	Body         string
	// ConfirmOtherCounterparty is the user's answer to "this chat shows
	// someone else". Without it such a chat is refused.
	ConfirmOtherCounterparty bool
	// WriteAs is the principal the activity is written under. For the
	// reading's own text it is the reader acting for the user. Nil keeps the
	// user, who typed or changed the text.
	WriteAs func(context.Context) context.Context
}

// refuseUnaskableQuestion refuses a chat capture of a file that is not an
// image. The text lane would otherwise read any text file as a "chat".
func refuseUnaskableQuestion(ctx context.Context, tx pgx.Tx, attachmentID ids.UUID, question string) error {
	switch question {
	case ExtractionQuestionDealFacts:
		return nil
	case ExtractionQuestionChatCapture:
	default:
		return &ChatCaptureError{
			Field: "question", Code: "unknown_question",
			Message: fmt.Sprintf("%q is not a question a file can be read for", question),
		}
	}
	var contentType *string
	if err := tx.QueryRow(ctx, `SELECT content_type FROM attachment WHERE id = $1`, attachmentID).
		Scan(&contentType); err != nil {
		return fmt.Errorf("activities: reading a file's type: %w", err)
	}
	if contentType == nil || !slices.Contains(ChatImageMIMEs(), bareMediaType(*contentType)) {
		return &ChatCaptureError{
			Field: "question", Code: "not_an_image",
			Message: "a chat is read from a JPEG, PNG, GIF or WebP screenshot; this file is none of them",
		}
	}
	return nil
}

// bareMediaType drops a Content-Type's parameters, so `image/png; q=1` is a PNG.
func bareMediaType(contentType string) string {
	media, _, _ := strings.Cut(contentType, ";")
	return strings.ToLower(strings.TrimSpace(media))
}

// FileChatCapture writes the message activity and moves the image onto it, in
// one transaction. The gates run under the caller's own principal, before
// WriteAs is applied, because the reader's principal passes every RBAC check.
func (s *Store) FileChatCapture(ctx context.Context, in ChatCaptureFiling) (ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return ids.UUID{}, err
	}
	if utf8.RuneCountInString(in.Body) > MaxChatText {
		return ids.UUID{}, &ChatCaptureError{
			Field: fieldBody, Code: "too_long",
			Message: fmt.Sprintf("a chat holds at most %d characters; shorten the text", MaxChatText),
		}
	}
	var filed ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Locked before the parent is read, so a second confirm waits here and
		// then sees the file already on the first confirm's activity.
		if _, err := storekit.LockRow(ctx, tx, "attachment", in.AttachmentID, storekit.LiveOnly); err != nil {
			return err
		}
		parentType, err := resolveAttachmentParent(ctx, tx, in.AttachmentID, principal.ActionUpdate)
		if err != nil {
			return err
		}
		if parentType == linkEntityActivity {
			filed, err = refiledChat(ctx, tx, in)
			return err
		}
		if parentType != linkEntityContact {
			return &ChatCaptureError{
				Code:    "unsupported_entity_type",
				Message: "a chat is filed from a screenshot on a contact; this file hangs on a " + parentType,
			}
		}
		read, err := requireChatReading(ctx, tx, in)
		if err != nil {
			return err
		}
		if err := requireSameCounterparty(ctx, tx, in, read); err != nil {
			return err
		}
		if err := requireRegisteredChannel(ctx, tx, in.Channel); err != nil {
			return err
		}
		filed, err = s.writeChatActivity(ctx, tx, in)
		if err != nil {
			return err
		}
		return refileAttachment(ctx, tx, in.AttachmentID, filed)
	})
	return filed, err
}

// requireChatReading holds the confirm to the reading the user was shown: one
// of this file, read for a chat, and finished.
func requireChatReading(ctx context.Context, tx pgx.Tx, in ChatCaptureFiling) (ExtractionRead, error) {
	read, err := scanExtractionRead(tx.QueryRow(ctx,
		`SELECT `+extractionReadColumns+` FROM attachment_extraction WHERE id = $1 AND attachment_id = $2`,
		in.ReadingID, in.AttachmentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return read, fmt.Errorf("%w: extraction reading %s", apperrors.ErrNotFound, in.ReadingID)
	}
	if err != nil {
		return read, fmt.Errorf("activities: reading the chat reading: %w", err)
	}
	if read.Question != ExtractionQuestionChatCapture {
		return read, &ChatCaptureError{
			Field: "extraction_id", Code: "not_a_chat_reading",
			Message: "this reading looked for deal facts, not for a chat",
		}
	}
	if read.Live() {
		return read, &ChatCaptureError{
			Field: "extraction_id", Code: "reading_not_finished",
			Message: "the screenshot is still being read; file it when the reading finishes",
		}
	}
	return read, nil
}

// requireSameCounterparty refuses a chat whose header names someone other
// than the contact, unless the user confirmed it belongs here anyway.
func requireSameCounterparty(ctx context.Context, tx pgx.Tx, in ChatCaptureFiling, read ExtractionRead) error {
	if in.ConfirmOtherCounterparty {
		return nil
	}
	contactID, err := attachmentParentID(ctx, tx, in.AttachmentID)
	if err != nil {
		return err
	}
	shows, err := otherChatCounterparty(ctx, tx, contactID, read)
	if err != nil || shows == "" {
		return err
	}
	return fmt.Errorf("%w: this chat shows %s, not this contact; confirm to file it here anyway",
		apperrors.ErrConflict, shows)
}

// requireRegisteredChannel checks the channel against the registry, so a
// provider added by migration is offered here with nothing else to change.
func requireRegisteredChannel(ctx context.Context, tx pgx.Tx, channel string) error {
	var registered bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM channel_provider WHERE provider = $1)`, channel).Scan(&registered); err != nil {
		return fmt.Errorf("activities: reading the channel registry: %w", err)
	}
	if !registered {
		return &ChatCaptureError{
			Field: "channel_provider", Code: "provider_not_registered",
			Message: fmt.Sprintf("%q is not a channel this installation knows; pick one from the list", channel),
		}
	}
	return nil
}

func (s *Store) writeChatActivity(ctx context.Context, tx pgx.Tx, in ChatCaptureFiling) (ids.UUID, error) {
	contactID, err := attachmentParentID(ctx, tx, in.AttachmentID)
	if err != nil {
		return ids.UUID{}, err
	}
	source := ChatCaptureSourceSystem
	sourceID := in.ReadingID.String()
	var body *string
	if strings.TrimSpace(in.Body) != "" {
		body = &in.Body
	}
	writeCtx := ctx
	if in.WriteAs != nil {
		writeCtx = in.WriteAs(ctx)
	}
	out, _, err := s.LogActivityTx(writeCtx, tx, LogActivityInput{
		Kind:            string(crmcontracts.ActivityKindMessage),
		ChannelProvider: in.Channel,
		Body:            body,
		Links:           []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: contactID}},
		SourceSystem:    &source,
		SourceID:        &sourceID,
		Source:          ChatCaptureSourceSystem,
	})
	if err != nil {
		return ids.UUID{}, err
	}
	return ids.UUID(out.Id), nil
}

// refiledChat answers a confirm repeated after the image already moved. It
// returns the activity the first confirm wrote. A file on any other activity
// is refused.
func refiledChat(ctx context.Context, tx pgx.Tx, in ChatCaptureFiling) (ids.UUID, error) {
	var filed ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.id FROM attachment at
		  JOIN activity a ON a.id = at.entity_id
		 WHERE at.id = $1 AND a.source_system = $2 AND a.source_id = $3`,
		in.AttachmentID, ChatCaptureSourceSystem, in.ReadingID.String()).Scan(&filed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, &ChatCaptureError{
			Code:    "unsupported_entity_type",
			Message: "this file is already filed on an activity",
		}
	}
	return filed, err
}

func attachmentParentID(ctx context.Context, tx pgx.Tx, attachmentID ids.UUID) (ids.UUID, error) {
	var parent ids.UUID
	if err := tx.QueryRow(ctx, `SELECT entity_id FROM attachment WHERE id = $1`, attachmentID).Scan(&parent); err != nil {
		return ids.UUID{}, fmt.Errorf("activities: reading a file's record: %w", err)
	}
	return parent, nil
}

// refileAttachment moves a file onto an activity. The row keeps its id, bytes
// and checksum, so the reading of it stays attached and erasure of the
// activity reaches the image.
func refileAttachment(ctx context.Context, tx pgx.Tx, attachmentID, activityID ids.UUID) error {
	if _, err := storekit.LockRow(ctx, tx, "attachment", attachmentID, storekit.LiveOnly); err != nil {
		return err
	}
	before, err := readAttachment(ctx, tx, attachmentID)
	if err != nil {
		return err
	}
	rollUp, hasAccount, err := accountRollUp(ctx, tx, linkEntityActivity, activityID)
	if err != nil {
		return err
	}
	var account *ids.UUID
	if hasAccount {
		account = &rollUp
	}
	if _, err := tx.Exec(ctx,
		`UPDATE attachment SET entity_type = $2, entity_id = $3, company_id = $4 WHERE id = $1`,
		attachmentID, linkEntityActivity, activityID, account); err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "attachment", attachmentID,
		map[string]any{fieldEntityType: string(before.EntityType), fieldEntityID: before.EntityId.String()},
		map[string]any{fieldEntityType: linkEntityActivity, fieldEntityID: activityID.String()})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, attachmentID, crmcontracts.PublicEventAttachmentUpdated{
		ParentType:  linkEntityActivity,
		ParentId:    openapi_types.UUID(activityID),
		ContentType: before.ContentType,
		ByteSize:    before.ByteSize,
	})
}
