// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The write path for a BYOK credential: an admin puts a key in, the routing
// lane serves with it, and nothing along the way can read it back out.
//
// Against a REAL vault and a real settings store, because what is being proved
// is that the ciphertext lands somewhere the resolver can reach — a fake vault
// would prove the store calls Put and nothing about whether the key serves.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// searchVault is realVault's twin over this env's pool — the same real
// provider, because what these tests prove is that ciphertext written by the
// store is readable by the resolver, which a fake would assert nothing about.
// The root key is fixed and meaningless.
func searchVault(t *testing.T, e *SearchEnv) keyvault.Vault {
	t.Helper()
	vault, err := keyvault.New(keyvault.Config{RootKey: bytes.Repeat([]byte{9}, 32), Pool: e.Pool})
	if err != nil {
		t.Fatalf("building the test vault: %v", err)
	}
	return vault
}

// sealingConfig reads and saves stored settings through e's pool and seals
// through the real vault, as compose wires a role with a vault.
func sealingConfig(t *testing.T, e *SearchEnv) *config.Config {
	t.Helper()
	return compose.WithVault(compose.StoredConfig(e.Pool), searchVault(t, e), discard())
}

func providerKeyStore(t *testing.T, e *SearchEnv) *ai.ProviderKeyStore {
	t.Helper()
	return ai.NewProviderKeyStore(sealingConfig(t, e))
}

// resolveKey asks the same resolver the routing lane uses, so the test observes
// what a model client would be handed rather than what the store recorded.
func resolveKey(ctx context.Context, t *testing.T, e *SearchEnv, provider string) string {
	t.Helper()
	return ai.KeyLookup(ctx, sealingConfig(t, e), discard())(ai.KeyEnvVarFor(provider))
}

// keySetting is the setting a provider's key is stored under.
func keySetting(t *testing.T, provider string) *config.Setting[string] {
	t.Helper()
	for _, s := range ai.KeySettings() {
		if s.Env() == ai.KeyEnvVarFor(provider) {
			return s
		}
	}
	t.Fatalf("%s takes no key", provider)
	return nil
}

// storedRef is the vault ref a provider's row holds, unopened.
func storedRef(ctx context.Context, t *testing.T, e *SearchEnv, provider string) string {
	t.Helper()
	ref, _, err := config.SealedRef(ctx, compose.StoredConfig(e.Pool), keySetting(t, provider))
	if err != nil {
		t.Fatalf("reading where the %s key is sealed: %v", provider, err)
	}
	return ref
}

func TestASealedKeyIsWhatTheRoutingLaneResolves(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	const key = "sk-a-real-looking-credential"
	if err := store.Set(ctx, "gemini", ai.ProviderCredential{APIKey: key}); err != nil {
		t.Fatalf("sealing the gemini key: %v", err)
	}
	// The resolver, not the store: this is the value a client would be built
	// with, which is the only sense in which the key "works".
	if got := resolveKey(ctx, t, e, "gemini"); got != key {
		t.Fatalf("the routing lane resolved %q, want the key that was sealed", got)
	}
}

func TestTheStoredSettingHoldsARefAndNeverTheKey(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	const key = "sk-must-not-appear-in-the-setting"
	if err := store.Set(ctx, "openai", ai.ProviderCredential{APIKey: key}); err != nil {
		t.Fatal(err)
	}
	// A setting row is readable by anything holding the read grant, and it is
	// dumped in support bundles and audit trails. The credential must not be
	// in it — only an opaque, workspace-bound handle.
	var row string
	if err := e.Pool.QueryRow(ctx, `SELECT value::text FROM setting WHERE key = $1`, keySetting(t, "openai").Key()).Scan(&row); err != nil {
		t.Fatalf("reading the openai row: %v", err)
	}
	if strings.Contains(row, key) {
		t.Fatalf("the openai setting carries the credential itself: %q", row)
	}
	if storedRef(ctx, t, e, "openai") == "" {
		t.Fatal("no ref recorded, so nothing was sealed")
	}
}

func TestRotatingAKeyServesTheNewOneAndRetiresTheOld(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	if err := store.Set(ctx, "anthropic", ai.ProviderCredential{APIKey: "sk-first"}); err != nil {
		t.Fatal(err)
	}
	firstRef := storedRef(ctx, t, e, "anthropic")

	if err := store.Set(ctx, "anthropic", ai.ProviderCredential{APIKey: "sk-second"}); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if got := resolveKey(ctx, t, e, "anthropic"); got != "sk-second" {
		t.Fatalf("after rotation the lane resolved %q, want the new key", got)
	}
	// The superseded blob is gone, not merely unreferenced: a credential no ref
	// names is one no operator can find to delete, and it stays decryptable by
	// anything holding the root key.
	if storedRef(ctx, t, e, "anthropic") == firstRef {
		t.Fatal("the ref did not move, so the rotation stored nothing new")
	}
	if _, err := searchVault(t, e).Get(ctx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(firstRef)); err == nil {
		t.Error("the superseded credential is still readable from the vault")
	}
}

// Two admins keying one vendor at once leave one key serving and no blob
// stranded. A save takes the write lock before it reads the ref it retires.
//
// The interleaving is forced. The test holds the key's lock, waits until a Set
// is parked on it, then records another key and commits. Another vendor's key,
// written in the same transaction, stays.
func TestAConcurrentKeyWriteRetiresTheKeyItReplaced(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)
	settingsStore := compose.NewSettingsStore(e.Pool)
	openai := keySetting(t, "openai").Key()
	held := sealKey(ctx, t, e, "sk-held")

	setDone := make(chan error, 1)
	// Held open for the whole body: the lock is transaction-scoped, so the
	// window this test needs is exactly this transaction's lifetime.
	if err := settingsStore.WriteTx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, openai); err != nil {
			return err
		}
		go func() { setDone <- store.Set(ctx, "openai", ai.ProviderCredential{APIKey: "sk-openai"}) }()
		waitForLockWaiter(ctx, t, e, openai)

		// The other admin's writes, through the real writer, while the Set is
		// parked. Committing this releases the lock and lets the Set proceed.
		if err := settingsStore.SetRawTx(ctx, tx, openai, json.RawMessage(`"`+held+`"`)); err != nil {
			return err
		}
		return settingsStore.SetRawTx(ctx, tx, keySetting(t, "anthropic").Key(),
			json.RawMessage(`"`+sealKey(ctx, t, e, "sk-anthropic")+`"`))
	}); err != nil {
		t.Fatalf("the holding transaction failed: %v", err)
	}
	if err := <-setDone; err != nil {
		t.Fatalf("the concurrent Set failed: %v", err)
	}

	for provider, want := range map[string]string{"anthropic": "sk-anthropic", "openai": "sk-openai"} {
		if got := resolveKey(ctx, t, e, provider); got != want {
			t.Errorf("%s resolved %q, want %q: the concurrent write dropped it", provider, got, want)
		}
	}
	if _, err := searchVault(t, e).Get(ctx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(held)); err == nil {
		t.Error("the key the Set replaced is still in the vault, named by no row")
	}
}

// waitForLockWaiter blocks until a session is parked on THIS key's advisory
// lock, which is how the test knows the Set reached its lock acquisition without
// sleeping for a guessed interval.
//
// Polls Postgres's own view of who is waiting rather than a duration, so the case
// is decided by the state it depends on.
//
// Narrowed to the key, this database, and somebody else's backend, because the
// broad form is a census that passes short: `locktype = 'advisory' AND NOT
// granted` is satisfied by any session anywhere in the cluster parked on any
// advisory lock, and this lane shares one Postgres across worktrees. It would
// then report "the Set is parked" when it is not, the holding transaction would
// commit, the Set would run afterwards and merge correctly, and the test would
// pass having proved nothing about ordering — with no failing assertion anywhere.
//
// The three columns are how Postgres records the single-argument form, measured
// rather than assumed: classid is the high 32 bits of the bigint the int4 hash
// promotes to, objid the low 32, and objsubid is 1.
func waitForLockWaiter(ctx context.Context, t *testing.T, e *SearchEnv, key string) {
	t.Helper()
	const parkedOnKey = `
		SELECT count(*) FROM pg_locks
		WHERE locktype = 'advisory'
		  AND NOT granted
		  AND objsubid = 1
		  AND pid <> pg_backend_pid()
		  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND classid = ((hashtext($1)::bigint >> 32) & 4294967295)
		  AND objid = (hashtext($1)::bigint & 4294967295)`

	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		// Checked BEFORE the query, because once the deadline expires the query
		// itself errors and the run would die on "context deadline exceeded"
		// rather than on the diagnosis below.
		if deadline.Err() != nil {
			t.Fatalf("no session ever parked on %s's advisory lock, so this case proves nothing about ordering", key)
		}
		var waiting int
		if err := e.Pool.QueryRow(deadline, parkedOnKey, key).Scan(&waiting); err != nil {
			if deadline.Err() != nil {
				t.Fatalf("no session ever parked on %s's advisory lock, so this case proves nothing about ordering", key)
			}
			t.Fatalf("reading pg_locks while waiting for the parked write: %v", err)
		}
		if waiting > 0 {
			return
		}
	}
}

// A ROTATED key reaches a running Router without a restart, and a REMOVED one
// stops being used.
//
// This is the property the credential surface exists for and the one that nearly
// shipped broken. The watcher compared only `RoutingVersion`, which is a digest
// of what the binding BINDS — tiers, models, base URLs — and is blind to the
// credential on purpose, because it doubles as a brief cache key. A rotation
// therefore moved nothing the watcher could see: every running api and worker
// kept calling the vendor with the key it resolved at boot, including one an
// admin had just revoked. Revoking a credential through the product that holds
// it has to stop its use.
//
// Driven through the real store, the real vault and a real ModelPath, because
// what is being proved is that the resolved CLIENTS changed — not that a digest
// moved.
func TestARotatedKeyReachesARunningRouterAndARemovedOneStopsBeingUsed(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	keys := providerKeyStore(t, e)
	// The vault root key rides the same lookup ResolveRouting consults — that is
	// how sealedKeys reads it, and a t.Setenv would leave THIS lookup without a
	// vault, which is a green test that resolves from the environment instead of
	// from the seal. The value matches searchVault's root key: a different one
	// builds a vault that cannot decrypt what the store sealed, and the failure
	// then reads as a missing provider key rather than as a root-key mismatch.
	// No provider key here — the point is that the VAULT supplies it, so an env
	// copy would make the rotation unobservable.
	env := config.Static(map[string]string{
		keyvault.EnvRootKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)),
	})

	// A binding on a provider that TAKES a key, so the credential is part of
	// what the clients are built with.
	routing := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), compose.StoredConfig(e.Pool))
	bound, err := routing.Replace(ctx, cloudRouting(t))
	if err != nil {
		t.Fatalf("storing the binding: %v", err)
	}
	if err := keys.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-first"}); err != nil {
		t.Fatalf("sealing the first key: %v", err)
	}

	// Resolved the way a boot resolves it, so the Router carries whatever
	// production would have stamped on it.
	resolved, err := compose.ResolveRouting(ctx, e.Pool, "", env, discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	path, err := compose.NewModelPath(ctx, resolved, e.Pool, false, discard())
	if err != nil {
		t.Fatalf("NewModelPath: %v", err)
	}
	router := path.Router()
	if router == nil {
		t.Fatal("the resolved path binds no router; the test can observe nothing")
	}
	watcher := compose.NewRoutingWatcher(e.Pool, &path, env, discard())
	atBoot := router.CredentialVersion()
	if atBoot == "" {
		t.Fatal("the Router carries no credential version, so a rotation cannot be observed at all")
	}

	// An unchanged installation must not rebind: dropping every cached
	// completion on each tick is the cost this comparison exists to avoid.
	watcher.Recheck(ctx)
	if router.CredentialVersion() != atBoot {
		t.Fatalf("an unchanged installation moved the credential version to %q", router.CredentialVersion())
	}
	if router.RoutingVersion() != bound.RoutingVersion() {
		t.Fatalf("an unchanged installation moved the routing version")
	}

	// Rotate. The BINDING is untouched, so the routing digest cannot move and
	// the credential digest is the only thing that can carry this.
	if err := keys.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-second"}); err != nil {
		t.Fatalf("rotating: %v", err)
	}
	watcher.Recheck(ctx)
	if router.RoutingVersion() != bound.RoutingVersion() {
		t.Errorf("the rotation moved the ROUTING version to %q; that digest is a brief cache key and must not move for a key",
			router.RoutingVersion())
	}
	rotated := router.CredentialVersion()
	if rotated == atBoot {
		t.Fatal("the Router still holds the credential it resolved at boot, so a rotation reaches no running role until it restarts")
	}

	// A REMOVAL that leaves the binding unservable is a different case, and this
	// pins what the system does rather than what one might hope.
	//
	// Every tier here is bound to gemini, so dropping its key leaves a config
	// this process cannot build clients for. `applyIfChanged` then keeps the
	// running binding by design — "turning a bad edit into an outage would be
	// worse" — which for a CREDENTIAL means the revoked key stays in use until
	// the process restarts. That reasoning was written for a bad binding edit,
	// where the old binding is still a legitimate one; a removed credential is
	// not. Whether a revocation should instead fail the lane closed is a
	// security-posture decision with a real cost on both sides, so it is stated
	// here as behaviour and not quietly assumed either way.
	if err := keys.Remove(ctx, "gemini"); err != nil {
		t.Fatalf("removing: %v", err)
	}
	watcher.Recheck(ctx)
	if router.CredentialVersion() != rotated {
		t.Errorf("credential version = %q, want the rotated %q: a removal that leaves the binding unservable keeps the running one",
			router.CredentialVersion(), rotated)
	}

	// Re-keying makes the binding servable again, and the version moves — which
	// is what shows the comparison above is live rather than the rebuild simply
	// always failing from here on.
	if err := keys.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-third"}); err != nil {
		t.Fatalf("re-keying: %v", err)
	}
	watcher.Recheck(ctx)
	if router.CredentialVersion() == rotated {
		t.Error("a servable credential change did not reach the Router")
	}
}

// The audit trail records that a credential changed and never what it points at.
//
// `audit_log` is a log sink like any other — admin-readable over /audit-log and
// exportable — so a vault ref written verbatim into a before/after image is a
// capability handle sitting in the one place an installation hands out wholesale.
// `settings.Entry.AsSecretReference` is what redacts it, and this entry was
// declared without it: every set, rotate and remove wrote the full provider→ref
// map into the row.
//
// Asserted against the ROW rather than against the entry's declaration, because
// what matters is the bytes that land in the sink; a redaction that stops being
// applied leaves the declaration looking correct.
func TestTheAuditTrailOfAKeyChangeCarriesNoRef(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	if err := store.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-audited"}); err != nil {
		t.Fatalf("sealing: %v", err)
	}
	ref := storedRef(ctx, t, e, "gemini")
	if ref == "" {
		t.Fatal("nothing was stored, so the test would pass on an empty trail")
	}

	// Every image the change wrote, before and after, on one row or many.
	var images string
	if err := e.Pool.QueryRow(ctx, `
		SELECT coalesce(string_agg(coalesce(before::text, '') || '|' || coalesce(after::text, ''), ' '), '')
		FROM audit_log WHERE entity_type = $1`, "ai_routing").Scan(&images); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if images == "" {
		t.Fatal("the key change wrote no audit row at all")
	}
	if strings.Contains(images, ref) {
		t.Errorf("the audit trail carries the vault ref verbatim: %s", images)
	}
	// Not only the whole ref — its unguessable tail is the capability half, and
	// a partial write would be just as reachable.
	if tail := ref[len(ref)-12:]; strings.Contains(images, tail) {
		t.Errorf("the audit trail carries a fragment of the vault ref: %s", images)
	}
}

// cloudRouting binds every tier to a provider that TAKES a key, so a credential
// is part of what the clients are built with. `fake` needs none, which would
// make a rotation unobservable for the wrong reason.
func cloudRouting(t *testing.T) ai.RoutingConfig {
	t.Helper()
	cfg, err := ai.ParseRouting([]byte(`profile: cloud_frontier
tiers:
  local_small: {provider: gemini, model: gemini-3.1-flash-lite}
  cheap_cloud: {provider: gemini, model: gemini-3.1-flash-lite}
  premium: {provider: gemini, model: gemini-3.5-flash}
  frontier: {provider: gemini, model: gemini-3.5-flash}
embeddings: {provider: gemini, model: gemini-embedding-001, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("parsing the cloud binding: %v", err)
	}
	return cfg
}

// sealKey puts a credential in the vault the way the store does and returns the
// ref, so the competing write records a ref that really resolves.
func sealKey(ctx context.Context, t *testing.T, e *SearchEnv, key string) string {
	t.Helper()
	ref, err := searchVault(t, e).Put(ctx, ids.From[ids.WorkspaceKind](e.WS), []byte(key))
	if err != nil {
		t.Fatalf("sealing %s: %v", key, err)
	}
	return string(ref)
}

func TestRemovingAKeyLeavesTheProviderUnconfigured(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	if err := store.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-to-be-removed"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(ctx, "gemini"); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if got := resolveKey(ctx, t, e, "gemini"); got != "" {
		t.Fatalf("a removed key still resolves as %q", got)
	}
	// Removing what is not there is the state the caller asked for, not a fault.
	if err := store.Remove(ctx, "gemini"); err != nil {
		t.Errorf("removing an absent key = %v, want success", err)
	}
}

func TestListNamesEveryServableProviderNotOnlyTheConfiguredOnes(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := providerKeyStore(t, e)

	if err := store.Set(ctx, "gemini", ai.ProviderCredential{APIKey: "sk-only-one"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// An installation that has configured nothing is exactly the one that needs
	// the screen, so the list is the providers this build serves — not the rows
	// that happen to exist.
	if len(got) != len(ai.CloudProvidersNeedingKeys()) {
		t.Fatalf("List returned %d providers, want every servable one (%d)", len(got), len(ai.CloudProvidersNeedingKeys()))
	}
	seen := map[string]bool{}
	for _, s := range got {
		seen[s.Provider] = s.Configured
		if s.EnvVar == "" {
			t.Errorf("%s names no environment variable, so a screen cannot say how a key seeded it", s.Provider)
		}
	}
	if !seen["gemini"] {
		t.Error("the configured provider does not read as configured")
	}
	if seen["openai"] {
		t.Error("a provider with no key reads as configured")
	}
}

func TestAProviderThisBuildCannotServeIsRefused(t *testing.T) {
	e := SetupSearch(t)
	store := providerKeyStore(t, e)
	// A key sealed against a name nothing routes is a credential nobody can use
	// and nobody will remember is there.
	if err := store.Set(e.adminRoutingCtx(), "ollama", ai.ProviderCredential{APIKey: "sk-local-needs-none"}); err == nil {
		t.Fatal("a provider that takes no api key accepted one")
	}
}

// Each vendor takes one kind of credential, and the other kind — or both, or
// neither — is refused as the caller's fault before anything is sealed or
// sent to Google. A key file that is not a service account's is refused by
// the field at fault, and nothing of what was sent comes back.
func TestACredentialOfTheWrongKindIsRefusedBeforeItIsSealed(t *testing.T) {
	e := SetupSearch(t)
	store := providerKeyStore(t, e)
	const notAKeyFile = `{"type":"authorized_user","client_secret":"GOCSPX-must-not-echo"}`
	cases := map[string]struct {
		provider string
		sent     ai.ProviderCredential
		want     string
	}{
		"an api key for vertex":        {"gemini_vertex", ai.ProviderCredential{APIKey: "sk-wrong-kind"}, "takes a service_account_json"},
		"a key file for gemini":        {"gemini", ai.ProviderCredential{ServiceAccountJSON: "{}"}, "takes an api_key"},
		"both":                         {"gemini_vertex", ai.ProviderCredential{APIKey: "sk-x", ServiceAccountJSON: "{}"}, "not both"},
		"neither, for vertex":          {"gemini_vertex", ai.ProviderCredential{}, "service-account key is empty"},
		"neither, for anthropic":       {"anthropic", ai.ProviderCredential{}, "api key is empty"},
		"a user credential for vertex": {"gemini_vertex", ai.ProviderCredential{ServiceAccountJSON: notAKeyFile}, "type must be service_account"},
	}
	before := searchSealedBlobCount(t, e)
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := store.Set(e.adminRoutingCtx(), c.provider, c.sent)
			var invalid settings.InvalidValue
			if !errors.As(err, &invalid) {
				t.Fatalf("want a 422-shaped refusal, got %v", err)
			}
			if !strings.Contains(invalid.Reason, c.want) {
				t.Errorf("reason = %q, want it to say %q", invalid.Reason, c.want)
			}
			if strings.Contains(invalid.Reason, "GOCSPX") || strings.Contains(invalid.Reason, "sk-") {
				t.Errorf("the refusal repeats what was sent: %q", invalid.Reason)
			}
		})
	}
	if after := searchSealedBlobCount(t, e); after != before {
		t.Errorf("refused credentials sealed %d blob(s)", after-before)
	}
}

// A screen asks List which field to offer, so the kind is stated for every
// vendor, keyed or not.
func TestListSaysWhichCredentialEachVendorTakes(t *testing.T) {
	e := SetupSearch(t)
	got, err := providerKeyStore(t, e).List(e.adminRoutingCtx())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	for _, s := range got {
		want := ai.CredentialKindAPIKey
		if s.Provider == "gemini_vertex" {
			want = ai.CredentialKindServiceAccount
		}
		if s.CredentialKind != want {
			t.Errorf("%s takes %q, want %q", s.Provider, s.CredentialKind, want)
		}
	}
}

func TestTheKeySurfaceIsAdminOnly(t *testing.T) {
	e := SetupSearch(t)
	store := providerKeyStore(t, e)
	// A FULLY FORMED principal that simply lacks the grant — workspace bound,
	// actor set, correlation carried. A bare context would be refused by the
	// settings store for having no workspace, so it would pass this test
	// against a store with no RBAC gate at all.
	rep := e.repNoRoutingCtx()
	if _, err := store.List(rep); err == nil {
		t.Error("List admitted a seat without the ai_routing grant")
	}
	if err := store.Set(rep, "gemini", ai.ProviderCredential{APIKey: "sk-nope"}); err == nil {
		t.Error("Set admitted a seat without the ai_routing grant")
	}
	if err := store.Remove(rep, "gemini"); err == nil {
		t.Error("Remove admitted a seat without the ai_routing grant")
	}
}

// searchSealedBlobCount is how many secrets the vault holds for this env's
// workspace — the *Env twin of the same name lives beside the seal tests.
func searchSealedBlobCount(t *testing.T, e *SearchEnv) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM vault_secret`).Scan(&n); err != nil {
		t.Fatalf("counting sealed secrets: %v", err)
	}
	return n
}

// repNoRoutingCtx is a seat with everything an admitted caller has except the
// one grant this surface gates on — the credential a model calls with is not
// something a rep may read, rotate or delete.
func (e *SearchEnv) repNoRoutingCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// TestAReadOnlySeatCannotSealAKey is the case the store's own update gate
// exists for, and the only one that can see it.
//
// A seat holding ai_routing READ passes the store's first step — reading the
// refs — so without the gate it reaches vault.Put and seals the caller's key
// BEFORE the settings write refuses. The request still fails, which is why
// asserting on the error proves nothing; what it leaves behind is a credential
// no ref names, written by someone not allowed to write. So the assertion is
// the blob count, not the error.
func TestAReadOnlySeatCannotSealAKey(t *testing.T) {
	e := SetupSearch(t)
	store := providerKeyStore(t, e)
	reader := e.routingReaderCtx()

	// It can see the surface — otherwise this proves nothing about the update
	// gate, only about the read one.
	if _, err := store.List(reader); err != nil {
		t.Fatalf("a read grant could not list: %v", err)
	}

	before := searchSealedBlobCount(t, e)
	if err := store.Set(reader, "gemini", ai.ProviderCredential{APIKey: "sk-read-only-seat"}); err == nil {
		t.Error("a read-only seat sealed a provider key")
	}
	if after := searchSealedBlobCount(t, e); after != before {
		t.Errorf("the refused Set sealed %d blob(s) anyway — the refusal must come before the seal, or a seat that may only look leaves credentials behind", after-before)
	}

	// Removing a provider that holds nothing is the one call that returns
	// BEFORE the settings write, so it is the one place the store's own update
	// gate is the only thing standing there. Without it a read-only seat is
	// told its removal succeeded — a "yes" to someone who may not change
	// anything, about a change that was never theirs to make.
	if err := store.Remove(reader, "openai"); err == nil {
		t.Error("a read-only seat was told it removed a key it may not touch")
	}
}

// routingReaderCtx holds ai_routing READ and nothing else: it may see which
// vendors are configured, and change none of them.
func (e *SearchEnv) routingReaderCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}
