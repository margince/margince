// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A claim read from a date-only note carries that precision, so a client draws
// its source's date with no time. The write reads no source time and says instant.
func TestAClaimCarriesThePrecisionOfItsSource(t *testing.T) {
	e := integration.Setup(t)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Precision contact", &e.AdminUser))
	quote := "I will send the rollout plan."
	day := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	writer := activities.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))).WithRecordZone(identity.ZoneAppliedTx)
	source, _, err := writer.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Body: &quote, OccurredAt: &day,
		OccurredPrecision: activities.PrecisionDay, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	filed := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: contact, Kind: "commitment_ours", Body: quote,
		ActivityID: ids.UUID(source.Id), Quote: quote, Source: "manual",
	})
	if filed.OccurredAt != nil || filed.OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
		t.Errorf("filed claim at %v as %q, want no time and instant", filed.OccurredAt, filed.OccurredPrecision)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		claims, err := e.Contacts.ClaimsForContact(e.Admin(), tx, contact, nil, 20)
		if err != nil {
			return err
		}
		if len(claims) != 1 || claims[0].OccurredAt == nil ||
			claims[0].OccurredPrecision != crmcontracts.OccurredPrecisionOccurredPrecisionDay {
			t.Errorf("claims = %+v, want one read at its source's day", claims)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
