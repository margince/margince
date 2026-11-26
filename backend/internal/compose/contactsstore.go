// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/compose/employerchange"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

func newContactsStore(db *database.DB) *contacts.Store {
	return contacts.NewStore(db).WithConsumerMail(capture.MatcherTx).WithFloorCarry(EntityFloorCarry(db.Pool())).
		WithSenderStamp(activities.StampProvenSendersTx).WithEmployerChangeProposer(employerchange.Proposer{DB: db, Inbox: approvalsInbox{}})
}

// approvalsInbox sits behind an interface: NewApprovalsService is built from a store.
type approvalsInbox struct{}

func (approvalsInbox) Service(db *database.DB) *approvals.Service { return NewApprovalsService(db) }
