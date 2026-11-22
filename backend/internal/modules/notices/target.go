// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/targettype"
)

// OwnTargetType is a notice target type that an approval cannot name. The notice
// points at a pending decision, a correction run, or retained content a privacy
// review asks someone to check. A notice may also point at anything an approval
// can, so the schema CHECK on notice.target_type is held equal to this set plus
// targettype.Type.
//
// Held by: TestNoticeTargetTypeIsTheApprovalListAndTheNoticeOwn (backend/gates/approvaltargettypes_test.go)
type OwnTargetType string

const (
	OwnTargetApproval               OwnTargetType = "approval"
	OwnTargetDataTask               OwnTargetType = "data_task"
	OwnTargetRetainedEmailSignature OwnTargetType = "retained_email_signature"
	OwnTargetEmailSignature         OwnTargetType = "email_signature"
	OwnTargetSignatureTemplate      OwnTargetType = "signature_template"
	OwnTargetLegalEntity            OwnTargetType = "legal_entity"
	OwnTargetKnowledgeDocument      OwnTargetType = "knowledge_document"
	OwnTargetApprovalHistory        OwnTargetType = "approval_history"
)

var ownTargets = map[string]struct{}{
	string(OwnTargetApproval): {}, string(OwnTargetDataTask): {}, string(OwnTargetRetainedEmailSignature): {},
	string(OwnTargetEmailSignature): {}, string(OwnTargetSignatureTemplate): {}, string(OwnTargetLegalEntity): {},
	string(OwnTargetKnowledgeDocument): {}, string(OwnTargetApprovalHistory): {},
}

// requireKnownTarget refuses a target type the schema CHECK would refuse.
// It also holds the list where the CHECK was dropped because an old row broke
// it. A notice about no record has no type to check.
func requireKnownTarget(target Target) error {
	if !target.Named() {
		return nil
	}
	if _, own := ownTargets[target.Type]; own || targettype.Known(target.Type) {
		return nil
	}
	return fmt.Errorf("notices: target type %q is not one a notice can point at: %w", target.Type, apperrors.ErrInvalidArgument)
}

// TargetApproval is the target type of a notice that opens a pending decision.
const TargetApproval = string(OwnTargetApproval)

// TargetDataTask is the target type of a notice about one background
// correction run; a client opens System health on it.
const TargetDataTask = string(OwnTargetDataTask)

// TargetRetainedEmailSignature is the target type of a notice about a departed
// member's signature. The frontend routes this value to the review screen.
const TargetRetainedEmailSignature = string(OwnTargetRetainedEmailSignature)

// Target is the record a notice is about, when it is about one.
//
// Both halves or neither, as the table's own constraint holds. A type with no
// id names a kind of thing and cannot be opened. An id with no type cannot be
// routed to a screen. The zero value is a notice about no record, such as a
// capture backlog or a coach's word, and most notices are.
type Target struct {
	Type string
	ID   ids.UUID
}

// Named reports whether this notice points at a record.
func (t Target) Named() bool { return t.Type != "" && !t.ID.IsZero() }

// Earlier retained-signature notices carry their references in the descriptor.
// Resolving the link keeps their stored text and delivery identity intact.
func resolvedNoticeTarget(stored Target, kind string, message *systemtext.Message) Target {
	if stored.Named() || kind != KindPrivacyReviewRequired || message == nil || message.Version != 1 || message.Body == nil {
		return stored
	}
	body := message.Body
	if body.Code != "privacy.review.inactive" || len(body.Args) != 2 || body.Args[0].Kind != "text" || body.Args[1].Kind != "text" {
		return stored
	}
	signature, err := ids.Parse(body.Args[0].Text)
	if err != nil || signature.IsZero() {
		return stored
	}
	owner, err := ids.Parse(body.Args[1].Text)
	if err != nil || owner.IsZero() {
		return stored
	}
	return Target{Type: TargetRetainedEmailSignature, ID: signature}
}
