// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// The dead-address derivation against rows the real writers produced. The
// rules under test are SQL — recipient attribution, the bystander guard on
// legacy rows, the clean-delivery clear — so a unit double proves none of
// them.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (e *storeEnv) deadFor(t *testing.T, addresses ...string) map[string]time.Time {
	t.Helper()
	var dead map[string]time.Time
	err := database.WithWorkspaceTx(readerCtx(e.ws, e.user), e.store.db.Pool(), func(tx pgx.Tx) error {
		var txErr error
		dead, txErr = e.store.DeadAddressesTx(readerCtx(e.ws, e.user), tx, addresses)
		return txErr
	})
	if err != nil {
		t.Fatalf("DeadAddressesTx: %v", err)
	}
	return dead
}

func TestADeadAddressIsTheOneTheReportNamedAndAWorkingSendRevivesIt(t *testing.T) {
	e := setupStore(t)

	// The suite's standard send carries a CC; the report names only the
	// primary recipient, and the CC'd bystander on the same row must not be
	// marked.
	id := e.stage(t, e.baseInput(e.activity, "pair@myco.test"))
	if err := e.store.RecordSent(e.ctx, id, connector.SendReceipt{ProviderMessageID: "prov-pair"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	if marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID),
		bounceFor("pair@myco.test", connector.BounceHard, "550 user unknown")); err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}

	dead := e.deadFor(t, "buyer@example.com", "cc@example.com")
	if _, marked := dead["buyer@example.com"]; !marked {
		t.Fatalf("the address the report named is not dead: %v", dead)
	}
	if _, marked := dead["cc@example.com"]; marked {
		t.Fatal("the CC'd bystander on the bounced send was marked dead")
	}

	// A later clean delivery to the same address revives it with no writer:
	// the derivation compares instants, and the newest one now says it works.
	e.clockValue = e.clockValue.Add(time.Hour)
	revive := e.stage(t, e.baseInput(e.activity2, "revive@myco.test"))
	if err := e.store.RecordSent(e.ctx, revive, connector.SendReceipt{ProviderMessageID: "prov-revive"}); err != nil {
		t.Fatalf("recording the reviving send: %v", err)
	}
	if dead := e.deadFor(t, "buyer@example.com"); len(dead) != 0 {
		t.Fatalf("a clean delivery after the bounce did not clear the mark: %v", dead)
	}
}

// A row stamped before bounce_recipient existed carries NULL: it may blame
// its recipient only when it has exactly one, so a legacy multi-recipient row
// marks nobody. The legacy state is produced by erasing the recorded
// recipient with the owner connection: RecordBounce, which stamps it, did not
// exist when those rows were written.
func TestALegacyBounceRowMarksOnlyASoleRecipient(t *testing.T) {
	e := setupStore(t)

	soloInput := e.baseInput(e.activity, "legacy-sole@myco.test")
	soloInput.Cc = []string{}
	sole := e.stage(t, soloInput)
	if err := e.store.RecordSent(e.ctx, sole, connector.SendReceipt{ProviderMessageID: "prov-ls"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	if marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID),
		bounceFor("legacy-sole@myco.test", connector.BounceHard, "550")); err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}
	pair := e.stage(t, e.baseInput(e.activity2, "legacy-pair@myco.test"))
	if err := e.store.RecordSent(e.ctx, pair, connector.SendReceipt{ProviderMessageID: "prov-lp"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	if marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID),
		bounceFor("legacy-pair@myco.test", connector.BounceHard, "550")); err != nil || !marked {
		t.Fatalf("recording the bounce: marked=%v err=%v", marked, err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE comms_outbound SET bounce_recipient = NULL WHERE id = ANY($1)`,
		[]ids.UUID{sole, pair}); err != nil {
		t.Fatalf("erasing the recorded recipients: %v", err)
	}

	dead := e.deadFor(t, "buyer@example.com", "cc@example.com")
	if _, marked := dead["buyer@example.com"]; !marked {
		t.Fatalf("a legacy sole-recipient bounce did not mark its one address: %v", dead)
	}
	if _, marked := dead["cc@example.com"]; marked {
		t.Fatal("a legacy multi-recipient bounce marked a bystander")
	}
}
