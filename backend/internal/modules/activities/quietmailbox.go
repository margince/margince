// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether the quiet-record scan may call a record silent.
//
// The timeline shows only what Margince captured. When the seat a reminder
// would go to has no mailbox connected, or its history is still being
// imported, a record that seat wrote to yesterday reads as silent, and the
// reminder it earns is false. So the scan draws an owned record only while
// its owner's mail is visible.

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// OwnerMailbox is how the scan asks whether Margince can see one seat's mail.
// capture owns the answer (capture.MailboxCaughtUpSQL) and compose hands it
// over, because this module cannot import capture.
type OwnerMailbox struct {
	// CaughtUp renders the condition for a SQL expression naming a user id,
	// with Providers bound at providersPos.
	CaughtUp func(userExpr string, providersPos int) string
	// Providers are the providers that connect a mailbox. A calendar carries
	// no mail, so connecting one proves nothing about silence.
	Providers []string
}

// WithOwnerMailbox returns a store whose quiet-record scan skips an owned
// record while its owner's mail is not visible. It returns a copy so the base
// store stays unchanged.
//
// A store without it scans as it did before the check existed, because it has
// no way to ask capture. The time scanner's store (compose/timescan.go) wires
// it; other stores never run the scan.
func (s *Store) WithOwnerMailbox(m OwnerMailbox) *Store {
	clone := *s
	clone.ownerMailbox = &m
	return &clone
}

// quietRecordOwner is the owner of the drawn row q, for each record type the
// scan draws. The tables are spelled out rather than formatted from the type,
// so the reader censuses in gates/ can see this read.
func quietRecordOwner() string {
	return storekit.SQLf(`CASE q.entity_type
		  WHEN '%s' THEN (SELECT r.owner_id FROM deal r WHERE r.id = q.entity_id)
		  WHEN '%s' THEN (SELECT r.owner_id FROM company r WHERE r.id = q.entity_id)
		  WHEN '%s' THEN (SELECT r.owner_id FROM contact r WHERE r.id = q.entity_id)
		  WHEN '%s' THEN (SELECT r.owner_id FROM lead r WHERE r.id = q.entity_id)
		END`,
		datasource.RecordDeal, datasource.RecordCompany, datasource.RecordContact, datasource.RecordLead)
}

// ownerMailboxUnseen renders an AND clause that drops a row whose owner's mail
// is not visible. An unowned row passes: its reminder goes to the unassigned
// queue, and there is no one seat whose mailbox would prove it wrong.
// Empty on a store without the check (WithOwnerMailbox).
func (m *OwnerMailbox) ownerMailboxUnseen(ownerExpr string, providersPos int) string {
	if m == nil {
		return ""
	}
	return storekit.SQLf(`
			  AND NOT EXISTS (
			    SELECT 1 FROM (SELECT %s AS owner_id) own
			    WHERE own.owner_id IS NOT NULL AND NOT %s)`,
		ownerExpr, m.CaughtUp("own.owner_id", providersPos))
}
