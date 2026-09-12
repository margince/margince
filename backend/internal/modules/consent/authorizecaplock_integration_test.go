// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The frequency cap's advisory lock actually serializes two dispatches.
//
// The lock is the whole defence: without it two workers holding the same
// address both read "one below the ceiling", both decide allow, and the cap is
// exceeded by exactly the number of concurrent dispatchers. The four tests
// beside this file derive lock KEYS and say themselves that they never open a
// second transaction, so the serialization was unproven.
//
// Held the way TestOneContactsConsentWritesDoNotInterleave holds its lock: take
// the same key from a separate session, drive the REAL authorization, and watch
// Postgres report it stopped. That is the mechanism rather than a race for the
// symptom — a test that launched two dispatches and hoped they crossed would
// have to lose a coin toss to fail, and would pass on the machine where it
// mattered least.

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// holdingSession opens a connection outside the pool, so a lock it takes is
// held while the code under test runs on a different backend.
func holdingSession(t *testing.T) *pgx.Conn {
	t.Helper()
	holder, err := pgx.Connect(context.Background(), os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("opening the holding connection: %v", err)
	}
	t.Cleanup(func() {
		if err := holder.Close(context.Background()); err != nil {
			t.Errorf("closing the holding connection: %v", err)
		}
	})
	return holder
}

// holdCapLock takes an address's cap lock from the holding session, spelled
// through capLockKey so it is the SAME key the dispatch takes. A lock on some
// other number would be a test of nothing.
//
// The session form, not the transaction form: it must outlive the statement
// that took it, because the whole point is to still be holding it while the
// dispatch runs.
func holdCapLock(t *testing.T, holder *pgx.Conn, address string) {
	t.Helper()
	if _, err := holder.Exec(context.Background(),
		`SELECT pg_advisory_lock($1)`, capLockKey(address)); err != nil {
		t.Fatalf("taking the cap lock for %s: %v", address, err)
	}
}

func releaseCapLock(t *testing.T, holder *pgx.Conn, address string) {
	t.Helper()
	if _, err := holder.Exec(context.Background(),
		`SELECT pg_advisory_unlock($1)`, capLockKey(address)); err != nil {
		t.Fatalf("releasing the cap lock for %s: %v", address, err)
	}
}

// capWindow is the window every ceiling in this file uses. The tests are about
// serialization, not about when a message ages out — that is the sibling's
// TestAMessageOlderThanTheWindowHasStoppedCounting — so one value keeps the
// window from reading as a variable any of these cases depends on.
const capWindow = 24 * time.Hour

// testJurisdiction hands out a code no other test is using.
//
// Handed out rather than chosen because the registry is process-wide and panics
// on a second rule set for a country it already knows — deliberately, since two
// would be two answers about one country's law, and there is no deregister. A
// hand-picked code meant every new cap test had to find a letter pair nobody
// had taken, across files, and a wrong guess panicked the whole package rather
// than failing one test.
//
// Codes are drawn IN ORDER from the QM-QZ range and each is taken at most once,
// so a collision is impossible rather than unlikely — hashing the test name
// into fourteen slots would have been four tests inside a space where a fifth
// has a fair chance of landing on one already used. ISO 3166 never assigns a
// QM-QZ code, so no jurisdiction this product serves can arrive under one, and
// the sibling file's "z" codes are a separate range again.
var capJurisdictions = struct {
	mu   sync.Mutex
	next int
}{}

// testJurisdictionSlots is how many codes the minter can hand out: q…a through
// q…z, minus the letters before "a" — the whole "q" prefix.
//
// WIDENED from the QM-QZ fourteen when the requirement tests began drawing from
// the same counter and the two together came within two of exhausting it. ISO
// 3166 reserves every QM-QZ code and assigns no other "q" code except QA
// (Qatar), which this skips: a test jurisdiction that collided with a real one
// would read as a pack for a country the product actually serves.
const testJurisdictionSlots = 25

func testJurisdiction(t *testing.T) jurisdiction.Code {
	t.Helper()
	capJurisdictions.mu.Lock()
	defer capJurisdictions.mu.Unlock()
	if capJurisdictions.next >= testJurisdictionSlots {
		t.Fatalf("this package has taken all %d test jurisdictions in the q… range — widen the "+
			"range rather than reusing one, which panics the registry for every test after it",
			testJurisdictionSlots)
	}
	// 'b' onward, skipping QA: Qatar is the one assigned "q" code.
	code := jurisdiction.Code("q" + string(rune('b'+capJurisdictions.next)))
	capJurisdictions.next++
	return code
}

// cappedGate registers a ceiling under a freshly minted jurisdiction and returns
// a gate bound to it.
func (e *capEnv) cappedGate(t *testing.T, messages int) *Gate {
	t.Helper()
	code := testJurisdiction(t)
	messagingrules.Register(messagingrules.Rules{
		Jurisdiction: code, Version: 1,
		FrequencyCap: &messagingrules.FrequencyCap{Messages: messages, Window: capWindow},
	})
	return NewGate(e.store).WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) {
			return code, nil
		}))
}

// grantMarketingTo gives a SECOND address a contact with a live marketing grant,
// so the engine reaches an allow for it and the lock is what a dispatch to it
// waits on — or does not. It reuses the purpose seedMarketingSubject created;
// calling that a second time would collide on the purpose key.
func (e *capEnv) grantMarketingTo(t *testing.T, address string) {
	t.Helper()
	ctx := context.Background()
	contactID := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Other Cap Subject', 'manual', 'human:x')`, contactID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, $2, true, 'manual', 'human:x')`, contactID, address); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO contact_consent (contact_id, purpose_id, state, lawful_basis, captured_at, source)
		SELECT $1, id, 'granted', 'consent', now(), 'test'
		  FROM consent_purpose WHERE key = $2`, contactID, marketingPurposeKey); err != nil {
		t.Fatal(err)
	}
}

// transmitFor is one advertising dispatch to this env's address.
func (e *capEnv) transmitFor(attempt int) commsauthz.TransmitRequest {
	return e.transmitForAddress(e.address, attempt)
}

// transmitForAddress is the same dispatch to any address, so a test can drive
// two recipients through the real gate at once.
func (e *capEnv) transmitForAddress(address string, attempt int) commsauthz.TransmitRequest {
	return commsauthz.TransmitRequest{
		DeliveryID: e.decisionDelivery, Attempt: attempt,
		Recipients: []connector.Recipient{{Email: address}},
		PurposeKey: marketingPurposeKey, Subject: "Offer", Body: "body",
	}
}

// A dispatch waits for an address's cap lock rather than counting past it.
//
// This is the property the ceiling rests on. With the lock held elsewhere the
// authorization must STOP — if it proceeds, two dispatchers can each read the
// count before the other's decision lands, and the ceiling is exceeded by
// however many are running.
func TestADispatchWaitsForTheAddressCapLock(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	gate := e.cappedGate(t, 3)

	holder := holdingSession(t)
	holdCapLock(t, holder, e.address)

	dispatched := make(chan error, 1)
	go func() {
		_, err := gate.AuthorizeTransmit(e.ctx, e.transmitFor(1))
		dispatched <- err
	}()

	waitUntilBlockedBy(t, holder)

	// Still stopped: nothing has released the lock. Asserted separately from
	// the wait, so a dispatch that never took the lock cannot pass by having
	// been granted it immediately.
	select {
	case err := <-dispatched:
		t.Fatalf("the dispatch finished while another session held this address's cap lock "+
			"(err %v) — the count and the decision are not serialized, so two dispatchers can "+
			"both read below the ceiling and both send", err)
	default:
	}

	releaseCapLock(t, holder, e.address)
	if err := <-dispatched; err != nil {
		t.Fatalf("the dispatch failed once the lock was free: %v", err)
	}
}

// A different address is not held behind this one.
//
// The control, and it is deliberately driven from BOTH sides through the real
// code. A dispatch to a second address runs while a dispatch to the first is
// stopped on its lock — so the lock being held is one production itself took,
// not one this test spelled. Holding a key of the test's own choosing would
// pass over a production lock taken on a constant: the two would simply never
// meet, and the test would report an independence it never observed.
//
// What it says: the key is derived from the address, and one busy mailbox does
// not serialize every advertising send in the installation.
func TestADispatchToAnotherAddressIsNotHeldBehindThisOne(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	gate := e.cappedGate(t, 3)

	// The first address's lock, held so a real dispatch to it stops and STAYS
	// stopped for the duration of the second.
	holder := holdingSession(t)
	holdCapLock(t, holder, e.address)
	released := false
	t.Cleanup(func() {
		if !released {
			releaseCapLock(t, holder, e.address)
		}
	})

	firstDone := make(chan error, 1)
	go func() {
		_, err := gate.AuthorizeTransmit(e.ctx, e.transmitFor(1))
		firstDone <- err
	}()
	waitUntilNBlockedBy(t, holder, 1)

	// A dispatch to a DIFFERENT address, while the first is still stopped. It
	// must not join the queue behind it.
	e.grantMarketingTo(t, "someone-else@cap.test")
	secondDone := make(chan error, 1)
	go func() {
		_, err := gate.AuthorizeTransmit(e.ctx, e.transmitForAddress("someone-else@cap.test", 1))
		secondDone <- err
	}()

	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("dispatching to an address nobody holds: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Error("a dispatch to an unrelated address blocked behind this one, so the cap lock is " +
			"not per-address and one busy mailbox serializes every advertising send there is")
	}

	releaseCapLock(t, holder, e.address)
	released = true
	if err := <-firstDone; err != nil {
		t.Fatalf("the first dispatch failed once its lock was free: %v", err)
	}
}

// The count happens AFTER the lock is held, not before it.
//
// This is the ordering the cap rests on, and it is the one the other tests in
// this file cannot see: they hold the lock externally and watch a dispatch stop
// at it, which proves the dispatch REACHES the lock but not that it reaches it
// before counting. Moving lockCapAddresses to after applyFrequencyCap leaves
// every one of them green.
//
// So this observes the order rather than the blocking. The lock is held, a
// dispatch stops on it, and a further advertising message is committed WHILE it
// waits. A dispatch that counts after acquiring must see that row and refuse; a
// dispatch that counted first is holding a number taken before the row existed
// and allows. The difference is the whole defect.
func TestTheCountIsTakenAfterTheLockNotBeforeIt(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	gate := e.cappedGate(t, 1)

	holder := holdingSession(t)
	holdCapLock(t, holder, e.address)
	released := false
	t.Cleanup(func() {
		if !released {
			releaseCapLock(t, holder, e.address)
		}
	})

	dispatched := make(chan bool, 1)
	go func() {
		ticket, err := gate.AuthorizeTransmit(e.ctx, e.transmitFor(1))
		if err != nil {
			t.Errorf("authorizing the dispatch: %v", err)
			dispatched <- true // an error must not read as a refusal
			return
		}
		dispatched <- ticket.Allowed
	}()

	waitUntilBlockedBy(t, holder)

	// The ceiling fills while the dispatch waits. Committed from another
	// session, so it is visible to any transaction that reads after this
	// point — and invisible to one that already read.
	e.deliveredAdvertising(t, time.Now().Add(-time.Minute))

	releaseCapLock(t, holder, e.address)
	released = true

	if <-dispatched {
		t.Error("the dispatch was ticketed although the ceiling filled while it waited for the " +
			"lock — it counted BEFORE acquiring, so the lock is serializing nothing that matters " +
			"and two dispatchers can both send on a number neither of them re-read")
	}
}

// Two dispatches for one address cannot both take the LAST slot under a ceiling.
//
// The property in the cap's own terms rather than the lock's, and the fixture
// is the whole test. An earlier version seeded the ceiling as already REACHED,
// so both dispatches denied for want of headroom and the assertion could not
// fail from a lock-ordering bug at all: moving the lock to AFTER the count left
// it green. It distinguished "the cap works" from "the cap is off", which two
// other tests already say.
//
// So the ceiling is seeded one BELOW. Exactly one slot is left, the correct
// answer is a split — one allow, one deny — and two allows is precisely what an
// unserialized pair produces, each having read the same count before the
// other's decision landed.
//
// The interleaving is arranged rather than raced: both dispatches are brought to
// a stop on the address's lock, held from another session, and only then is it
// released. Two goroutines merely launched together almost never cross tightly
// enough, and a version that hoped they would passed cleanly with the lock
// deleted.
func TestTwoConcurrentDispatchesCannotBothTakeTheLastSlot(t *testing.T) {
	e := setupCap(t)
	e.seedMarketingSubject(t)
	gate := e.cappedGate(t, 2)

	// One of two slots already spent, so one dispatch may go and the other
	// may not.
	e.deliveredAdvertising(t, time.Now().Add(-time.Minute))

	holder := holdingSession(t)
	holdCapLock(t, holder, e.address)
	released := false
	t.Cleanup(func() {
		if !released {
			releaseCapLock(t, holder, e.address)
		}
	})

	type outcome struct {
		allowed bool
		err     error
	}
	results := make(chan outcome, 2)
	dispatch := func(attempt int) {
		go func() {
			ticket, err := gate.AuthorizeTransmit(e.ctx, e.transmitFor(attempt))
			results <- outcome{allowed: ticket.Allowed, err: err}
		}()
	}

	// BOTH are waited for by COUNT, not by "somebody is blocked": the second
	// wait would otherwise return on the first dispatch's block, and the lock
	// would be released while dispatch 2 had not yet run a statement — the two
	// would then race exactly as this arrangement exists to prevent.
	dispatch(1)
	waitUntilNBlockedBy(t, holder, 1)
	dispatch(2)
	waitUntilNBlockedBy(t, holder, 2)
	releaseCapLock(t, holder, e.address)
	released = true

	allowed := 0
	for range 2 {
		got := <-results
		// Reported from the test goroutine: a t.Fatal inside a spawned one
		// stops that goroutine and lets the test pass regardless.
		if got.err != nil {
			t.Errorf("authorizing a concurrent dispatch: %v", got.err)
			continue
		}
		if got.allowed {
			allowed++
		}
	}
	if allowed != 1 {
		t.Errorf("%d of 2 concurrent dispatches were ticketed for the ONE slot left under the "+
			"ceiling, want exactly 1 — two means each read the count before the other's decision "+
			"landed, and the ceiling is exceeded by however many dispatchers are running", allowed)
	}
}
