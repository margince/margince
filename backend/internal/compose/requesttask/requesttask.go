// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package requesttask writes the task a data-subject request owes under a pack
// with deadlines of its own (consent/requesttask.go). It writes through the
// activities store that owns tasks. The task goes to the first privacy
// operator, the colleagues the missed-receipt alarm reaches, and is filed on
// the subject's record.
package requesttask

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const actor = "system:privacy-request-task"

// operatorObject is what a privacy operator must read: the rights cases.
const operatorObject = "privacy_request"

// Writer writes request tasks through the activities store.
type Writer struct {
	Tasks *activities.Store
}

var _ consent.RequestTasks = Writer{}

// OpenRequestTaskTx writes one request's task on the caller's transaction, in
// the installation's language.
func (w Writer) OpenRequestTaskTx(ctx context.Context, tx pgx.Tx, task consent.RequestTask) error {
	writer := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: actor})
	operators, err := identity.PrivacyReviewOperatorsTx(writer, tx, operatorObject)
	if err != nil {
		return err
	}
	var assignee *ids.UserID
	if len(operators) > 0 {
		assignee = &operators[0]
	}
	lang := identity.BaseLanguageForRecord(writer, tx)
	subject, body := words(task)
	subjectLine, err := subject.Render(lang)
	if err != nil {
		return err
	}
	bodyLine, err := body.Render(lang)
	if err != nil {
		return err
	}
	source, key := provenance.PrivacyRequestTaskSource, task.Right+":"+task.Key
	due := task.DueAt
	in := activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindTask), Subject: &subjectLine, Body: &bodyLine,
		SystemText:   &systemtext.Message{Version: 1, Subject: subject, Body: body},
		SourceSystem: &source, SourceID: &key, Source: source,
		DueAt: &due, AssigneeID: assignee, Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{{EntityType: task.SubjectType, EntityID: task.SubjectID}},
	}
	if _, _, err := w.Tasks.LogActivityTx(writer, tx, in); err != nil {
		return fmt.Errorf("requesttask: writing the task a data-subject request owes: %w", err)
	}
	return nil
}

// words names the work and its dates. The task's own due date is the date to
// carry it out. The reply date, when the pack sets one, is in the body.
func words(task consent.RequestTask) (*systemtext.Component, *systemtext.Component) {
	code := "privacy.request.stop"
	switch task.Right {
	case consent.RequestTaskRectify:
		code = "privacy.request.rectify"
	case consent.RequestTaskErasure:
		code = "privacy.request.erasure"
	}
	due := systemtext.Date(task.DueAt.UTC().Format(time.DateOnly))
	if task.ReplyBy.IsZero() {
		return systemtext.Sentence(code), systemtext.Sentence("privacy.request.due", due)
	}
	reply := systemtext.Date(task.ReplyBy.UTC().Format(time.DateOnly))
	return systemtext.Sentence(code), systemtext.Sentence("privacy.request.reply_and_due", reply, due)
}
