// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whose correspondence a captured chat message is, decided by whose credential
// carried it.
//
// A transport that spends ONE MEMBER's credential is that member's own account:
// their chats are correspondence, and the rules mail has always been under —
// the workspace floor, the seat's counterparty holds, the sender's own marker —
// reach them. A transport that spends the INSTALLATION's credential serves
// everybody, so its traffic is workspace business and none of those rules
// applies; holding such a row would leave it readable by nobody, because there
// is no member to hold it FOR.
//
// The control is the sharpest one available: the SAME unit, the same ingest, the
// same record, on two transports that differ only in the credential model they
// declared. Anything else would hold the behaviour to core-versus-unit, which is
// the derivation the declaration exists to replace.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// floorBotProvider is the control transport: the same unit's, declared as one
// credential for the whole installation.
const floorBotProvider = "probe_bot"

// setupFloorIngress composes the probe unit supplying BOTH credential models and
// registers both transports, so a test can put one record on either and change
// nothing else.
func setupFloorIngress(t *testing.T) *ingressEnv {
	t.Helper()
	e := setupExtRuntime(t)
	composeCapturingUnit(t, ingressUnit,
		[]extension.Channel{
			{Provider: ingressProbeProvider, CredentialModel: extension.CredentialPerMember},
			{Provider: floorBotProvider, CredentialModel: extension.CredentialWorkspaceBot},
		},
		extension.IngressSource{
			System: ingressProbeSystem, Lands: []extension.RecordKind{extension.KindActivity},
		})
	bindCaptureForTest(t, e)
	grantCapture(t, e, e.Rep1)
	depositCredential(t, e, e.Rep1)
	env := &ingressEnv{extRuntimeEnv: e, member: e.Rep1}
	registerFloorTransports(t, env)
	return env
}

// registerFloorTransports runs the real boot reconcile and takes both of the
// unit's transports away afterwards. It is registerProbeTransport's twin and
// differs only in clearing two providers instead of one; sharing that one would
// leave the control transport's registry row behind for every later test in the
// process to trip over.
func registerFloorTransports(t *testing.T, e *ingressEnv) {
	t.Helper()
	t.Cleanup(func() {
		activities.SetChannelProviders([]string{capture.ProviderTelegram})
		comms.SetChannelProviders([]string{capture.ProviderTelegram})
	})
	if err := reconcileChannelProviders(context.Background(), e.Pool, []string{capture.ProviderTelegram}); err != nil {
		t.Fatalf("registering the unit's transports: %v", err)
	}
	t.Cleanup(func() {
		owner := integration.OwnerConn(t)
		for _, provider := range []string{ingressProbeProvider, floorBotProvider} {
			// In dependency order: the registry row is the foreign-key parent of
			// every message filed on it and every identity bound on it.
			for _, statement := range []string{
				`DELETE FROM activity WHERE channel_provider = $1`,
				`DELETE FROM contact_channel_identity WHERE provider = $1`,
				`DELETE FROM channel_provider WHERE provider = $1`,
			} {
				if _, err := owner.Exec(context.Background(), statement, provider); err != nil {
					t.Errorf("cleaning up after %s (%s): %v", provider, statement, err)
				}
			}
		}
	})
}

// setFloor flips the workspace mail-sharing posture through the real settings
// store, so the fixture exercises the value the sink actually reads rather than
// a row this test wrote by hand.
func setFloor(t *testing.T, e *ingressEnv, sharing bool) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := capture.NewSettings(NewSettingsStore(e.Pool)).
		Update(ctx, capture.SettingsPatch{MailSharing: &sharing}); err != nil {
		t.Fatalf("setting mail sharing to %v: %v", sharing, err)
	}
}

// bornAudience reads what one landed message was born as.
func (e *ingressEnv) bornAudience(t *testing.T, activityID ids.UUID) (audience, reason string) {
	t.Helper()
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT audience, coalesce(audience_reason, '') FROM activity WHERE id = $1`,
			activityID).Scan(&audience, &reason)
	})
	return audience, reason
}

// hasAReader asks the audience gate's existential twin about the landed row —
// the invariant every hold in this file has to satisfy.
func (e *ingressEnv) hasAReader(t *testing.T, activityID ids.UUID) bool {
	t.Helper()
	var reachable bool
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		reachable, err = auth.ActivityHasAReaderTx(ctx, tx, activityID)
		return err
	})
	return reachable
}

// landOne ingests one record for the member and answers the activity it became.
func landOne(t *testing.T, e *ingressEnv, rec extension.Record) ids.UUID {
	t.Helper()
	result, err := e.ingestingRuntime().Ingest(context.Background(),
		extension.UserID(e.member.String()), rec)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if result.Disposition != extension.DispositionAccepted {
		t.Fatalf("disposition = %q, want accepted", result.Disposition)
	}
	return ids.MustParse(result.Ref.ID)
}

// The floor reaches a member's own chat.
//
// With mail sharing off the installation has said captured correspondence is not
// shared. A message that arrived on this member's own credential is exactly that,
// and it must be born held to the contacts on it rather than published to every
// seat — which is what it was before the birth ladder could read whose credential
// carried it.
func TestTheWorkspaceFloorHoldsAMemberBoundChannelMessage(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, false)

	id := landOne(t, e, aChannelMessage("ws-7:4001", "someone@gmail.com", "floor-1"))

	audience, reason := e.bornAudience(t, id)
	if audience != "participants" || reason != "workspace_floor" {
		t.Fatalf("born (%q, %q), want (participants, workspace_floor) — with the floor down a member's own chat is not workspace business",
			audience, reason)
	}
	// The hold is only legitimate because somebody can still open it. A held row
	// nobody can read is the defect this whole feature exists to refuse.
	if !e.hasAReader(t, id) {
		t.Fatal("the floor held the message and left it readable by nobody")
	}
}

// The same message, the same transport, with the floor UP: nothing to hold it,
// so it is born open. This is what holds the test above to the floor rather than
// to the credential model alone.
func TestAMemberBoundChannelMessageIsBornOpenWhileTheFloorIsUp(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, true)

	id := landOne(t, e, aChannelMessage("ws-7:4002", "someone@gmail.com", "floor-2"))

	if audience, reason := e.bornAudience(t, id); audience != "workspace" || reason != "" {
		t.Fatalf("born (%q, %q), want (workspace, \"\") — nothing asked for this message to be held", audience, reason)
	}
}

// The control: the floor does NOT reach a transport the installation shares.
//
// One record, one unit, one ingest — only the declared credential model differs.
// A workspace bot serves everybody, so its traffic is the company's own; and
// there is no member whose correspondence it could be held as, so a hold here
// would produce the unreadable row rather than a private one.
func TestTheFloorLeavesAWorkspaceBotsChannelMessageOpen(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, false)

	rec := aChannelMessage("ws-7:4003", "someone@gmail.com", "floor-3")
	rec.Activity.ChannelProvider = floorBotProvider
	rec.Counterparty.ChannelIdentity.Provider = floorBotProvider

	id := landOne(t, e, rec)

	if audience, reason := e.bornAudience(t, id); audience != "workspace" || reason != "" {
		t.Fatalf("born (%q, %q), want (workspace, \"\") — an installation's own bot carries workspace business, and holding it would leave the row with no reader at all",
			audience, reason)
	}
}

// A seat's standing decision about a correspondent reaches that correspondent's
// chat.
//
// A counterparty hold is stored per seat and asked of this seat, so a member-bound
// transport is exactly the case where it has somebody to be asked of. The record
// carries the addresses the hold is matched on, which is why this rung is not
// dead for a unit transport even though a core channel connector carries none.
func TestASeatsCounterpartyHoldReachesTheirOwnChat(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, true)
	holdCounterparty(t, e, "held@example.test")

	rec := aChannelMessage("ws-7:4004", "held@example.test", "floor-4")
	rec.Addresses = []string{"held@example.test"}
	id := landOne(t, e, rec)

	audience, reason := e.bornAudience(t, id)
	if audience != "participants" || reason != "counterparty" {
		t.Fatalf("born (%q, %q), want (participants, counterparty) — the seat asked for this correspondent's messages to be held",
			audience, reason)
	}
	if !e.hasAReader(t, id) {
		t.Fatal("the counterparty hold left the message readable by nobody")
	}
}

// holdCounterparty places one seat's hold on an address through the store the
// product writes it with — a hand-inserted row would prove nothing about the
// hold the seat's own click produces.
func holdCounterparty(t *testing.T, e *ingressEnv, address string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.member.String(), UserID: e.member,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"capture_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if _, err := capture.NewCounterpartyHoldStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))).
		Add(ctx, "address", address); err != nil {
		t.Fatalf("placing the seat's hold on %s: %v", address, err)
	}
}

// A sender who marked their own message is obeyed on a chat too.
func TestASendersOwnMarkerHoldsAMemberBoundChannelMessage(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, true)

	rec := aChannelMessage("ws-7:4005", "someone@gmail.com", "floor-5")
	rec.Activity.Subject = "Vertraulich: die Konditionen"
	id := landOne(t, e, rec)

	audience, reason := e.bornAudience(t, id)
	if audience != "participants" || reason != "explicitly_confidential" {
		t.Fatalf("born (%q, %q), want (participants, explicitly_confidential) — the sender said so themselves",
			audience, reason)
	}
	if !e.hasAReader(t, id) {
		t.Fatal("the sender's marker left the message readable by nobody")
	}
}

// The import row: what makes a hold on a chat message a contribution rather than
// a fact stranded on the row.
//
// Without it deriveAudienceTx answers "not a captured row" forever — the hold
// could never be re-derived across a later sync, listed among the member's held
// messages, or widened where a widening is the seat's to make. (The workspace
// floor is not: the widening pass matches a counterparty-only hold, and a floor
// an admin raised is nobody's to lift by hand.) The credential is the evidence:
// the
// ingress established, before capture ran, that this member deposited this
// unit's user-scoped secret and that the unit acted for them.
func TestAMemberBoundChannelMessageRecordsTheMembersImport(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, false)

	id := landOne(t, e, aChannelMessage("ws-7:4006", "someone@gmail.com", "floor-6"))

	var posture, reason string
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT coalesce(posture_at_import, ''), coalesce(verdict_reason, '')
			   FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
			id, e.member).Scan(&posture, &reason)
	})
	if posture != "held" || reason != "workspace_floor" {
		t.Fatalf("the member's import row reads (%q, %q), want (held, workspace_floor) — the hold has to be recorded where the derivation reads it",
			posture, reason)
	}

	// And the derivation reproduces the same answer from it, which is what makes
	// the hold survive every later sync of the same conversation.
	if err := recomputeAsWorkspace(t, e, id); err != nil {
		t.Fatalf("recomputing the audience: %v", err)
	}
	if audience, why := e.bornAudience(t, id); audience != "participants" || why != "workspace_floor" {
		t.Fatalf("after the recompute (%q, %q), want (participants, workspace_floor) — a derivation that cannot see the hold publishes it",
			audience, why)
	}
}

func recomputeAsWorkspace(t *testing.T, e *ingressEnv, id ids.UUID) error {
	t.Helper()
	var err error
	e.readAsWorkspace(t, func(ctx context.Context, tx pgx.Tx) error {
		err = activities.RecomputeAudienceTx(ctx, tx, ids.From[ids.ActivityKind](id))
		return nil
	})
	return err
}

// A workspace bot's message records no import row, because there is no member
// whose import it would be. The connecting admin is not that member: a channel
// connection's connected_by is audit-only.
func TestAWorkspaceBotsChannelMessageRecordsNoImport(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, false)

	rec := aChannelMessage("ws-7:4007", "someone@gmail.com", "floor-7")
	rec.Activity.ChannelProvider = floorBotProvider
	rec.Counterparty.ChannelIdentity.Provider = floorBotProvider
	id := landOne(t, e, rec)

	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM capture_import WHERE activity_id = $1`, id); got != 0 {
		t.Errorf("import rows = %d, want none — an installation's bot imports for nobody", got)
	}
}

// A second member of the same unit does not inherit the first member's held
// chat by re-ingesting its key.
//
// This is the bound on treating a credential as delivery evidence. On a
// per-member transport the core has no independent record of who a message was
// delivered to — the unit is the only witness — so a colleague who replays
// somebody else's source id is indistinguishable from one whose own credential
// genuinely carried it. Refusing costs the genuine colleague an import row they
// can capture for themselves; admitting would hand a guessed source id the
// content of a held conversation.
func TestASecondMembersReplayEarnsNoImportOnAHeldChat(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, false)
	grantCapture(t, e.extRuntimeEnv, e.Rep2)
	depositCredential(t, e.extRuntimeEnv, e.Rep2)

	record := aChannelMessage("ws-7:4008", "someone@gmail.com", "floor-8")
	id := landOne(t, e, record)

	// The same record, ingested for the OTHER member: a replay, because the
	// natural key already landed.
	if _, err := e.ingestingRuntime().Ingest(context.Background(),
		extension.UserID(e.Rep2.String()), record); err != nil {
		t.Fatalf("the second member's ingest: %v", err)
	}

	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
		id, e.Rep2); got != 0 {
		t.Errorf("the replaying member holds %d import rows, want none — a source id is not evidence that a credential carried the message", got)
	}
	// And the first member's import row is untouched, so the refusal is a refusal
	// rather than a rewrite of whose message it is.
	if got := e.countAsWorkspace(t,
		`SELECT count(*) FROM capture_import WHERE activity_id = $1 AND user_id = $2`,
		id, e.member); got != 1 {
		t.Errorf("the delivering member holds %d import rows, want exactly 1", got)
	}
}

// The consequence nobody asked for and everybody wanted: a member-bound chat's
// audience stops being a per-message answer.
//
// `refuseCapturedAudienceWrite` refuses a direct audience set on any row some
// seat imported, because such a row's audience is DERIVED from its importers.
// Giving a member-bound chat an import row therefore moves it onto the same
// footing as mail: the decision is made about the CONVERSATION, through the
// thread share/hold path that selects on capture_import, rather than message by
// message — which is how a thread ends up half shared.
//
// The workspace bot's message is the control, and it is the whole reason this
// is a pair rather than a claim: it has no import row, so the per-message write
// still reaches it. The refusal follows the import row, not the kind.
func TestAMemberBoundChatsAudienceIsNoLongerAPerMessageAnswer(t *testing.T) {
	e := setupFloorIngress(t)
	setFloor(t, e, true)

	mine := landOne(t, e, aChannelMessage("ws-7:4009", "someone@gmail.com", "floor-9"))

	bot := aChannelMessage("ws-7:4010", "someone@gmail.com", "floor-10")
	bot.Activity.ChannelProvider = floorBotProvider
	bot.Counterparty.ChannelIdentity.Provider = floorBotProvider
	theirs := landOne(t, e, bot)

	store := activities.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)))
	writer := e.audienceWriterCtx(t)

	_, err := store.SetAudience(writer, ids.From[ids.ActivityKind](mine),
		audienceNaming(e.member))
	var captured *activities.CapturedAudienceError
	if !errors.As(err, &captured) {
		t.Fatalf("setting a member-bound chat's audience answered %v, want the captured-audience refusal — "+
			"a row its importer's posture decides must not also be settable by hand, or the two writers disagree on the next sync", err)
	}

	if _, err := store.SetAudience(writer, ids.From[ids.ActivityKind](theirs),
		audienceNaming(e.member)); err != nil {
		t.Fatalf("setting a workspace bot's chat audience: %v — the refusal follows the import row, and a bot's message has none", err)
	}
}

// audienceNaming is a `selected` write naming one user — the narrowing both
// halves of the pair above are asked to make, so the only difference between
// them is the row it is asked of.
func audienceNaming(user ids.UUID) activities.SetAudienceInput {
	return activities.SetAudienceInput{
		Audience: "selected",
		Members:  []activities.AudienceMember{{SubjectType: "user", SubjectID: user}},
	}
}

// audienceWriterCtx binds the seat that reaches PATCH /activities/{id}/audience
// on the shipped path: a human on a full seat holding activity.update, on the
// widest row scope, so nothing about ROW authority is what the assertion above
// measures.
func (e *ingressEnv) audienceWriterCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.member.String(), UserID: e.member,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true},
				"contact":  {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}
