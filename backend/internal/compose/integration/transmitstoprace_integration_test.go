// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAStopWaitsForTheProviderHandoffAlreadyUnderway(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, stopDuringSend)
}

func TestAConsentWithdrawalWaitsForTheProviderHandoffAlreadyUnderway(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, withdrawalDuringSend)
}

func TestALeadCredentialStopOrdersWithAContactsSharedAddress(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, leadToDuringSend)
}

func TestABlindCopySharesItsAddressStopsHandoffFence(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, blindCopyBounceDuringSend)
}

func TestAConfirmationWaitsBeforeTakingTheSendingContactsRow(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, confirmationDuringSend)
}

type permissionWriteScenario uint8

const (
	stopDuringSend permissionWriteScenario = iota
	withdrawalDuringSend
	secondSendDuringStop
	leadToDuringSend
	blindCopyBounceDuringSend
	confirmationDuringSend
)

func assertPermissionWriteWaitsForHandoff(t *testing.T, scenario permissionWriteScenario) {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	path := "/v1/contacts/" + p.contactID + "/consent/suppress"
	write := AnyMap{"kind": "subject_request", "reason": "Please stop contacting me."}
	wantStatus := http.StatusNoContent
	if scenario == withdrawalDuringSend {
		var purpose string
		if err := p.Owner.QueryRow(t.Context(), "SELECT id FROM consent_purpose WHERE key = 'transactional'").Scan(&purpose); err != nil {
			t.Fatal(err)
		}
		path = "/v1/contacts/" + p.contactID + "/consent"
		write = AnyMap{"purpose_id": purpose, "new_state": "withdrawn"}
		wantStatus = http.StatusOK
	}
	if scenario == confirmationDuringSend {
		if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent/confirm-request", AnyMap{}, nil, nil); status != http.StatusCreated {
			t.Fatalf("request confirmation: %d", status)
		}
		path = "/v1/public/confirm/" + confirmLinkToken(t, p.AppEnv)
		write = AnyMap{"marketing_choice": "withdrawn"}
		wantStatus = http.StatusOK
	}
	var activity ids.UUID
	keys := []string{p.contactID}
	if scenario == leadToDuringSend || scenario == blindCopyBounceDuringSend {
		activity, path, keys = stageAddressStopMail(t, p, scenario)
		write = nil
		if scenario == leadToDuringSend {
			wantStatus = http.StatusOK
		}
	} else {
		activity = p.sendExpectingAcceptance(t, "transactional", "Requested update", "Here is the update.")
	}
	delivery, _ := p.deliveryFor(t, activity)
	var second ids.UUID
	if scenario == secondSendDuringStop {
		other := p.sendExpectingAcceptance(t, "transactional", "Another update", "A separate message.")
		second, _ = p.deliveryFor(t, other)
	}
	var captured sentMail
	provider := gmailSendStub(t, &captured, "")
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	barrier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/messages/send" {
			close(reached)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		provider.Config.Handler.ServeHTTP(w, r)
	}))
	defer barrier.Close()
	defer unblock()
	type dispatchResult struct {
		outcome comms.Outcome
		err     error
	}
	sent := make(chan dispatchResult, 1)
	go func() {
		outcome, _, err := p.dispatchThroughResult(t, delivery, barrier, nil)
		sent <- dispatchResult{outcome: outcome, err: err}
	}()
	waiting, cancel := context.WithTimeout(t.Context(), testdb.ProbeBudget)
	defer cancel()
	select {
	case <-reached:
	case result := <-sent:
		t.Fatalf("provider was never reached: %s: %v", result.outcome, result.err)
	case <-waiting.Done():
		t.Fatal("provider handoff did not start")
	}
	stopped, returned := make(chan int, 1), make(chan struct{})
	go func() {
		defer close(returned)
		switch scenario {
		case blindCopyBounceDuringSend:
			ctx := compose.SendWorkerContext(t.Context(), p.workspaceID(t))
			err := p.DB().Tx(ctx, func(tx pgx.Tx) error {
				return consent.RecordHardBounceTx(ctx, tx, consent.HardBounceFact{Address: "alias.buyer@preflight.test", DeliveryID: delivery})
			})
			if err != nil {
				t.Errorf("record delivery-report stop: %v", err)
				stopped <- http.StatusInternalServerError
			} else {
				stopped <- http.StatusNoContent
			}
		case leadToDuringSend:
			stopped <- publicCall(t, p.AppEnv, "POST", path, nil, nil, nil)
		case confirmationDuringSend:
			stopped <- publicCall(t, p.AppEnv, "POST", path, write, nil, nil)
		default:
			stopped <- p.Call(t, "POST", path, write, nil, nil)
		}
	}()
	holder := stopKeyHolder(t, p, keys)
	testdb.AwaitQueuedBehind(t, p.Owner, holder, 1, returned, "the stop")
	waitsOnStopKey(t, p, holder, keys)
	if scenario == confirmationDuringSend {
		probe, err := p.Owner.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var contact ids.UUID
		readErr := probe.QueryRow(t.Context(), "SELECT id FROM contact WHERE id=@id FOR UPDATE NOWAIT", pgx.NamedArgs{"id": p.contactID}).Scan(&contact)
		rollbackErr := probe.Rollback(t.Context())
		if err := errors.Join(readErr, rollbackErr); err != nil {
			t.Fatalf("confirmation took the contact row before waiting on its stop key: %v", err)
		}
	}
	if scenario == secondSendDuringStop {
		outcome, other, _ := p.dispatchOnce(t, second, "")
		if outcome != comms.OutcomePostponed || other.raw != "" {
			t.Fatalf("send while actual stop waits: %s", outcome)
		}
	}
	unblock()
	if result := <-sent; result.outcome != comms.OutcomeSent || result.err != nil {
		t.Fatalf("handoff outcome: %s: %v", result.outcome, result.err)
	}
	if status := <-stopped; status != wantStatus {
		t.Fatalf("record stop: %d", status)
	}
	if captured.raw == "" {
		t.Fatal("the ordered provider handoff transmitted no message")
	}
}

func TestAStopCommittedBeforeDispatchReachesNoProvider(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	activity := p.sendExpectingAcceptance(t, "transactional", "Requested update", "Here is the update.")
	delivery, _ := p.deliveryFor(t, activity)
	if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent/suppress", AnyMap{"kind": "subject_request", "reason": "Please stop contacting me."}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("record stop: %d", status)
	}
	outcome, captured, _ := p.dispatchOnce(t, delivery, "")
	if outcome == comms.OutcomeSent || captured.raw != "" {
		t.Fatalf("stopped delivery reached provider: %s", outcome)
	}
}

func TestAProviderFailureReleasesTheRecipientsStopFence(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	activity := p.sendExpectingAcceptance(t, "transactional", "Requested update", "Here is the update.")
	delivery, _ := p.deliveryFor(t, activity)
	var captured sentMail
	provider := gmailSendStub(t, &captured, "")
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/messages/send" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		provider.Config.Handler.ServeHTTP(w, r)
	}))
	defer refused.Close()
	outcome, _, sendErr := p.dispatchThroughResult(t, delivery, refused, nil)
	if outcome != comms.OutcomeRetry || !errors.Is(sendErr, connector.ErrUnreachable) {
		t.Fatalf("provider failure outcome: %s", outcome)
	}
	var locks int
	if err := p.Owner.QueryRow(t.Context(), `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`).Scan(&locks); err != nil {
		t.Fatal(err)
	}
	if locks != 0 {
		t.Fatalf("provider failure left %d advisory locks held", locks)
	}
	if captured.raw != "" {
		t.Fatal("the refusing provider accepted message bytes")
	}
}

func TestAConflictingPermissionWriterPostponesProviderHandoff(t *testing.T) {
	assertPermissionWriteWaitsForHandoff(t, secondSendDuringStop)
}

func TestSessionCapacityPostponesWithoutCallingTheProvider(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	activity := p.sendExpectingAcceptance(t, "transactional", "Requested update", "Here is the update.")
	delivery, _ := p.deliveryFor(t, activity)
	var captured sentMail
	provider := gmailSendStub(t, &captured, "")
	ctx := database.WithSessionBudget(compose.SendWorkerContext(t.Context(), p.workspaceID(t)), func(context.Context) (func(), error) { return nil, database.ErrSessionBudgetBusy })
	outcome, _, err := p.dispatchThroughContext(ctx, t, delivery, provider, nil)
	if err != nil || outcome != comms.OutcomePostponed || captured.raw != "" {
		t.Fatalf("capacity pressure: %s, %v, transmitted=%v", outcome, err, captured.raw != "")
	}
}

func TestReceiptReadBackDoesNotDelayARecordedStop(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	p.stakeADeal(t)
	activity := p.sendExpectingAcceptance(t, "transactional", "Requested update", "Here is the update.")
	delivery, _ := p.deliveryFor(t, activity)
	var captured sentMail
	provider := gmailSendStub(t, &captured, "")
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	delayed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/messages/") {
			close(reached)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		provider.Config.Handler.ServeHTTP(w, r)
	}))
	defer delayed.Close()
	defer unblock()
	finished := make(chan error, 1)
	go func() {
		outcome, _, err := p.dispatchThroughResult(t, delivery, delayed, nil)
		if err == nil && outcome != comms.OutcomeSent {
			err = fmt.Errorf("handoff outcome: %s", outcome)
		}
		finished <- err
	}()
	waiting, cancel := context.WithTimeout(t.Context(), testdb.ProbeBudget)
	defer cancel()
	select {
	case <-reached:
	case err := <-finished:
		t.Fatalf("no read-back: %v", err)
	case <-waiting.Done():
		t.Fatal("provider read-back did not start")
	}
	if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent/suppress", AnyMap{"kind": "subject_request", "reason": "Please stop contacting me."}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("stop during read-back: %d", status)
	}
	unblock()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if captured.raw == "" {
		t.Fatal("the provider had not accepted the message before read-back")
	}
}

func stageAddressStopMail(t *testing.T, p *preflightEnv, scenario permissionWriteScenario) (ids.UUID, string, []string) {
	t.Helper()
	const address = "alias.buyer@preflight.test"
	store := consent.NewStore(p.DB())
	ctx := compose.SendWorkerContext(t.Context(), p.workspaceID(t))
	var token string
	var lead string
	if scenario == leadToDuringSend {
		lead = createdID(t, p.AppEnv, "/v1/leads", AnyMap{"full_name": "Earlier lead", "email": address, "source": "manual"})
		if err := p.DB().Tx(ctx, func(tx pgx.Tx) error {
			var err error
			token, err = store.EnsureWithdrawalCredentialTx(ctx, tx, consent.WithdrawalMintInput{Address: address, LeadID: ids.From[ids.LeadKind](uuidOf(t, lead)), Scope: consent.WithdrawalScopeAllMarketing})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if status := p.Call(t, "DELETE", "/v1/leads/"+lead, AnyMap{}, nil, nil); status != http.StatusOK {
			t.Fatalf("archive earlier lead: %d", status)
		}
	}
	contact := createdID(t, p.AppEnv, "/v1/contacts", AnyMap{"full_name": "Later contact", "source": "manual", "emails": []AnyMap{{"email": address}}})
	body := AnyMap{"subject": "Product news", "body": "The requested news.", "to": []string{"ALIAS.BUYER@PREFLIGHT.TEST"}, "consent_purpose": "marketing_email", "communication_context": "marketing", "links": []AnyMap{{"entity_type": "contact", "entity_id": contact}}}
	path := ""
	if scenario == blindCopyBounceDuringSend {
		createdID(t, p.AppEnv, "/v1/relationships", AnyMap{"kind": "deal_stakeholder", "contact_id": contact, "deal_id": p.dealID, "role": "champion", "source": "manual"})
		apptest.AskedAboutTheDeal(t, p.AppEnv, contact, address, p.dealID)
		body["to"] = []string{"buyer@preflight.test"}
		body["bcc"] = []string{"ALIAS.BUYER@PREFLIGHT.TEST"}
		body["consent_purpose"] = "transactional"
		body["communication_context"] = "active_deal_followup"
		body["links"] = []AnyMap{{"entity_type": "contact", "entity_id": contact}, {"entity_type": "deal", "entity_id": p.dealID}}
	} else {
		ref, err := store.ResolveWithdrawalToken(ctx, token)
		if err != nil || ref.LeadID.UUID != uuidOf(t, lead) || !ref.ContactID.IsZero() {
			t.Fatalf("credential no longer names the earlier lead: %+v %v", ref, err)
		}
		recipient := *p
		recipient.contactID = contact
		recipient.grantMarketingConsent(t)
		path = "/v1/public/preferences/" + token + "/unsubscribe"
	}
	var activity struct {
		ID      string          `json:"id"`
		Code    string          `json:"code"`
		Detail  string          `json:"detail"`
		Details json.RawMessage `json:"details"`
	}
	if status := p.Call(t, "POST", "/v1/emails", body, nil, &activity); status != http.StatusAccepted {
		t.Fatalf("stage address-stop message: %d %s %s %s", status, activity.Code, activity.Detail, activity.Details)
	}
	return uuidOf(t, activity.ID), path, storekit.StopAddressLockKeys([]string{address}, nil)
}

// stopKeySQL matches the advisory lock row l when its key is one of @keys.
const stopKeySQL = `l.locktype = 'advisory'
 AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
 AND ((l.classid::bigint << 32) | (l.objid::bigint & 4294967295)) = ANY(ARRAY(
   SELECT hashtextextended(k, 0) FROM unnest(@keys::text[]) AS k))`

// stopKeyHolder names the backend that holds a stop key in share mode: the
// transmit, paused inside its provider handoff.
func stopKeyHolder(t *testing.T, p *preflightEnv, keys []string) int32 {
	t.Helper()
	var holders []int32
	if err := p.Owner.QueryRow(t.Context(), `SELECT coalesce(array_agg(DISTINCT l.pid), '{}') FROM pg_locks l
 WHERE l.granted AND l.mode = 'ShareLock' AND `+stopKeySQL, pgx.NamedArgs{"keys": keys}).Scan(&holders); err != nil {
		t.Fatalf("reading who holds the stop keys: %v", err)
	}
	if len(holders) != 1 {
		t.Fatalf("%d backends hold the stop keys in share mode, want the one transmit", len(holders))
	}
	return holders[0]
}

// waitsOnStopKey fails unless the session queued behind holder waits for a
// stop key, rather than for some other lock the transmit holds.
func waitsOnStopKey(t *testing.T, p *preflightEnv, holder int32, keys []string) {
	t.Helper()
	waiter, err := testdb.WaiterBehind(t.Context(), p.Owner, holder)
	if err != nil {
		t.Fatal(err)
	}
	var onKey bool
	if err := p.Owner.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_locks l
 WHERE l.pid = @waiter AND NOT l.granted AND `+stopKeySQL+`)`, pgx.NamedArgs{"waiter": waiter, "keys": keys}).Scan(&onKey); err != nil {
		t.Fatalf("reading what the stop waits for: %v", err)
	}
	if !onKey {
		t.Fatal("the stop waits on the transmit, but not for a stop key")
	}
}
