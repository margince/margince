// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package partnersteps writes a partner's next step as a task.
package partnersteps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/dueday"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// Tasks writes a partner's next step as the task the Partner tab's Add task
// writes. It is the task door's own request, on the partner's company. The
// caller holds it, or the human an agent acts for.
type Tasks struct{ tasks *activities.Store }

// New writes next steps through the activity store's task door.
func New(tasks *activities.Store) Tasks { return Tasks{tasks: tasks} }

// WriteTx writes the step's task inside the partner write's transaction.
func (p Tasks) WriteTx(ctx context.Context, tx pgx.Tx, step contacts.PartnerNextStep) error {
	links := []struct {
		EntityId   openapi_types.UUID                            `json:"entity_id"` //nolint:staticcheck // the generated link element spells it so, and this literal must match it
		EntityType crmcontracts.CreateTaskRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: openapi_types.UUID(step.CompanyID.UUID), EntityType: crmcontracts.CreateTaskRequestLinksEntityTypeCompany}}
	req := crmcontracts.CreateTaskRequest{Subject: step.Step, Links: &links, Source: "manual"}
	if step.Due != nil {
		// A due day ends in the installation's zone, as a promise's does.
		due, err := dueday.EndOf(ctx, tx, step.Due.Format(time.DateOnly), "")
		if err != nil {
			return err
		}
		req.DueAt = &due
	}
	in, err := activities.TaskInputFrom(req)
	if err != nil {
		return err
	}
	// Keyed on the company, the words and the day, so a retried partner write
	// finds the task it wrote rather than writing a second.
	source, key := partnerNextStepSource, partnerNextStepKey(step)
	in.SourceSystem, in.SourceID = &source, &key
	_, _, err = p.tasks.LogActivityTx(ctx, tx, in)
	return err
}

// partnerNextStepSource names the partner write as the task's origin.
const partnerNextStepSource = "partner-next-step"

func partnerNextStepKey(step contacts.PartnerNextStep) string {
	day := ""
	if step.Due != nil {
		day = step.Due.Format(time.DateOnly)
	}
	sum := sha256.Sum256([]byte(step.CompanyID.String() + "\x00" + step.Step + "\x00" + day))
	return hex.EncodeToString(sum[:])
}
