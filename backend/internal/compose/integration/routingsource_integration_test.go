// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Where the model binding comes from, end to end.
//
// Four things need a real database, and none of them can be shown with a unit
// test because each is about what the INSTALLATION holds rather than what this
// process was handed:
//
//   - a routing PATH binds nothing, however valid the file behind it and
//     however unconfigured the installation — the binding is a setting, and
//     `seeds.ai_routing` is the one thing that seeds it;
//   - a stored binding wins over any path, so an old --ai-routing left on a
//     command line cannot quietly re-point which vendor sees the text;
//   - a stored binding carries a routing VERSION, which is a cache key: read
//     back without one, every brief in the installation would fingerprint
//     against an empty string;
//   - an installation that binds nothing runs with its AI lanes absent rather
//     than failing the boot.

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// offlineRouting is a complete binding on the fake provider, so these tests
// need no credential and no network.
const offlineRouting = `profile: eu_hosted
tiers:
  local_small: {provider: fake, model: fake-small}
  cheap_cloud: {provider: fake, model: fake-small}
  premium: {provider: fake, model: fake-large}
  frontier: {provider: fake, model: fake-large}
embeddings: {provider: fake, model: fake-embed, dimensions: 8}
`

func writeRouting(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-routing.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing the routing file: %v", err)
	}
	return path
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

// A routing PATH is ignored. It used to seed an installation that had none,
// which made two files able to plant one binding — `seeds.ai_routing` in
// margince.yaml is the survivor, because it reaches every path that creates an
// installation instead of only a boot that finds the setting unset.
//
// The case is worth a database because "ignored" has to mean ignored even where
// the old arm was most tempting: nothing stored AND a perfectly valid file.
func TestAValidRoutingFileDoesNotBindAnInstallation(t *testing.T) {
	e := SetupSearch(t)
	path := writeRouting(t, offlineRouting)

	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, path, config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if !cfg.Unconfigured() {
		t.Fatalf("a routing path bound the installation to %+v; the binding is a stored setting and a file may not plant one", cfg.Tiers)
	}
}

// A path that cannot be read is ignored too, and that is the point rather than
// an oversight: the file is never opened, so there is nothing for a typo, a
// stale mount or a deleted file to break. Failing the boot would be the right
// answer only while the file is load-bearing, and it no longer is.
func TestAnUnreadableRoutingPathIsIgnored(t *testing.T) {
	e := SetupSearch(t)

	cfg, err := compose.ResolveRouting(context.Background(), e.Pool,
		filepath.Join(t.TempDir(), "does-not-exist.yaml"), config.Static(nil), discard())
	if err != nil {
		t.Fatalf("a missing routing path failed the boot: %v", err)
	}
	if !cfg.Unconfigured() {
		t.Error("a missing routing path produced a binding")
	}
}

// A stored binding is returned whatever the path says, including a path naming a
// DIFFERENT binding. This is the property an operator relies on when they leave
// an old --ai-routing on the command line: the flag cannot quietly re-point
// which vendor sees the installation's text.
func TestAStoredBindingWinsOverAnyRoutingPath(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	// Stored through the real writer — the one PUT /v1/ai/routing drives.
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "fake-stored")); err != nil {
		t.Fatalf("storing the binding: %v", err)
	}

	other := writeRouting(t, `profile: eu_hosted
tiers:
  local_small: {provider: fake, model: fake-other}
  cheap_cloud: {provider: fake, model: fake-other}
  premium: {provider: fake, model: fake-other}
  frontier: {provider: fake, model: fake-other}
embeddings: {provider: fake, model: fake-embed, dimensions: 8}
`)
	cfg, err := compose.ResolveRouting(ctx, e.Pool, other, config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if cfg.Unconfigured() {
		t.Fatal("the stored binding was not returned")
	}
	if got := cfg.Tiers["premium"].Model; got != "fake-stored" {
		t.Errorf("premium = %q, want the stored fake-stored: the routing path replaced the stored binding", got)
	}
}

// An installation that names no routing file and has nothing stored is not a
// boot error: its AI lanes are simply absent, exactly as before the move.
func TestAnInstallationThatBindsNothingResolvesUnconfigured(t *testing.T) {
	e := SetupSearch(t)

	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting with no file and nothing stored: %v", err)
	}
	if !cfg.Unconfigured() {
		t.Errorf("an installation that bound nothing resolved to %+v, want unconfigured", cfg.Tiers)
	}
}

// A stored binding is held to the same bar the file loader applies, so a write
// that reaches the row some other way cannot land something the boot would have
// refused. ai.FromStored is where that bar is applied on the way out.
func TestAStoredBindingIsValidatedOnTheWayOut(t *testing.T) {
	if _, err := ai.FromStored(ai.RoutingConfig{Profile: ai.ProfileEUHosted}, config.Static(nil)); err == nil {
		t.Error("a binding with no tiers finalized without error")
	}
}

// Replacing the binding through the store, and a running Router picking that up
// without a restart — the loop increments 5 and 3 exist to close together.
//
// A write surface without the re-read would be worse than neither: the UI would
// confirm a change the process kept ignoring, which is a disagreement nobody
// can see. So the write and the adoption are proved in one test rather than two.
func TestAReplacedBindingReachesARunningRouterWithoutARestart(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()

	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	first, err := store.Replace(ctx, parsedRouting(t, "first"))
	if err != nil {
		t.Fatalf("storing the first binding: %v", err)
	}

	// A real ModelPath, built the way a boot builds one, so the watcher is
	// handed exactly what production hands it.
	path, err := compose.NewModelPath(ctx, first, e.Pool, false, discard())
	if err != nil {
		t.Fatalf("NewModelPath: %v", err)
	}
	router := path.Router()
	if router == nil {
		t.Fatal("the resolved path binds no router; the test can observe nothing")
	}
	watcher := compose.NewRoutingWatcher(e.Pool, &path, config.Static(nil), discard())

	// A tick against an unchanged binding must leave the Router alone —
	// otherwise every cached completion is dropped every interval.
	watcher.Recheck(ctx)
	if router.RoutingVersion() != first.RoutingVersion() {
		t.Fatalf("an unchanged binding moved the Router to %q", router.RoutingVersion())
	}

	second, err := store.Replace(ctx, parsedRouting(t, "second"))
	if err != nil {
		t.Fatalf("replacing the binding: %v", err)
	}
	if second.RoutingVersion() == first.RoutingVersion() {
		t.Fatal("the two bindings share a version; the test cannot tell adoption from inaction")
	}

	watcher.Recheck(ctx)
	if router.RoutingVersion() != second.RoutingVersion() {
		t.Errorf("the Router still serves %q after the binding was replaced with %q",
			router.RoutingVersion(), second.RoutingVersion())
	}
	if m, ok := router.CurrentModelForTier(ai.TierPremium); !ok || m.Model != "second" {
		t.Errorf("premium = %+v ok=%v; the replacement did not reach the bound models", m, ok)
	}
}

// A binding the store refuses never becomes what anything serves. The bar is
// the one the routing file was always held to, applied on the way in rather
// than discovered at the first model call.
func TestTheStoreRefusesABindingTheFileLoaderWouldHaveRefused(t *testing.T) {
	e := SetupSearch(t)
	ctx := e.adminRoutingCtx()
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))

	if _, err := store.Replace(ctx, ai.RoutingConfig{
		Profile: "nowhere",
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierPremium: {Provider: "fake", Model: "m"}},
	}); err == nil {
		t.Fatal("an unknown profile was stored; a bad binding must be refused on the way in")
	}
	stored, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !stored.Unconfigured() {
		t.Errorf("the refused binding was stored anyway: %+v", stored.Tiers)
	}
}

// adminRoutingCtx is an admin holding the ai_routing grant the seeded role
// carries — read and update, no create or delete, which is what a setting has.
func (e *SearchEnv) adminRoutingCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

func parsedRouting(t *testing.T, model string) ai.RoutingConfig {
	t.Helper()
	cfg, err := ai.ParseRouting([]byte(`profile: eu_hosted
tiers:
  local_small: {provider: fake, model: ` + model + `}
  cheap_cloud: {provider: fake, model: ` + model + `}
  premium: {provider: fake, model: ` + model + `}
  frontier: {provider: fake, model: ` + model + `}
embeddings: {provider: fake, model: ` + model + `-embed, dimensions: 8}
`))
	if err != nil {
		t.Fatalf("ParseRouting: %v", err)
	}
	return cfg
}

// An UNCONFIGURED stored row is not the same as no row, and the difference is
// the whole reason the seed's answer is now read.
//
// An UNPROVISIONED installation announces the ignored path too.
//
// This is the boot with no workspace at all, and it returned before the warning
// — so the one operator most likely to be watching, having just started a fresh
// installation with the flag they have always passed, was the one told nothing.
// The wording has to name the actual situation as well: "no stored binding"
// would be misleading when there is no installation to hold one.
func TestAnUnprovisionedBootStillAnnouncesTheIgnoredRoutingPath(t *testing.T) {
	e := SetupSearch(t)
	// Archiving every workspace is what "unprovisioned" IS to this code path:
	// singletonWorkspace enumerates the LIVE ones, so none left means the claim
	// flow has not run. Same mechanism the claim suite uses.
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE workspace SET archived_at = now() WHERE archived_at IS NULL`); err != nil {
		t.Fatalf("clearing the harness company: %v", err)
	}

	var logged strings.Builder
	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, writeRouting(t, offlineRouting),
		config.Static(nil), slog.New(slog.NewTextHandler(&logged, nil)))
	if err != nil {
		t.Fatalf("an unprovisioned boot failed: %v", err)
	}
	if !cfg.Unconfigured() {
		t.Fatal("an unprovisioned installation produced a binding")
	}
	for _, want := range []string{"ignored", "not provisioned", "seeds.ai_routing"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the unprovisioned warning does not mention %q: %s", want, logged.String())
		}
	}
	// And it must not claim the installation merely has no binding — that sends
	// an operator to a settings screen no claim flow has opened yet.
	if strings.Contains(logged.String(), "NO stored model binding") {
		t.Errorf("the unprovisioned boot reported the wrong situation: %s", logged.String())
	}
}

// An ignored routing path is announced, and the two situations say different
// things. Asserting on the WORDS is the point: "ignored" alone, on a boot where
// the flag used to be what supplied the binding, leaves the operator to discover
// from a dead feature that their AI lanes are gone.
func TestAnIgnoredRoutingPathIsAnnouncedAndSaysWhichSituation(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()
	path := writeRouting(t, offlineRouting)

	// Nothing stored: this boot is the one where the lanes go absent.
	var unbound strings.Builder
	if _, err := compose.ResolveRouting(ctx, e.Pool, path, config.Static(nil),
		slog.New(slog.NewTextHandler(&unbound, nil))); err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	for _, want := range []string{"ignored", "NO stored model binding", "seeds.ai_routing"} {
		if !strings.Contains(unbound.String(), want) {
			t.Errorf("the unbound warning does not mention %q: %s", want, unbound.String())
		}
	}

	// Bound: the flag is merely redundant, and the warning must not claim the
	// lanes are absent — an operator who reads that goes looking for a fault
	// that is not there.
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "fake-stored")); err != nil {
		t.Fatalf("storing the binding: %v", err)
	}
	var bound strings.Builder
	if _, err := compose.ResolveRouting(ctx, e.Pool, path, config.Static(nil),
		slog.New(slog.NewTextHandler(&bound, nil))); err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if !strings.Contains(bound.String(), "ignored") {
		t.Errorf("a bound installation did not warn about the ignored flag: %s", bound.String())
	}
	if strings.Contains(bound.String(), "NO stored model binding") {
		t.Errorf("a bound installation was told its AI lanes are absent: %s", bound.String())
	}
}

// A key in the environment is sealed on a boot that has NO binding yet.
//
// The regression this closes was silent and self-defeating. Sealing lived
// BELOW the unconfigured early return, so a fresh installation — the only kind
// that has no binding — never reached it. `ai.provider_keys` stayed empty,
// which is the row /ai/provider-keys reports `configured` from, so Settings ->
// AI told an admin their vendor was unkeyed on the very screen they had opened
// to bind a tier to it. The key was in the environment the whole time and the
// api process had it.
//
// Sealing does not depend on a binding and never did: one is a credential
// moving out of the process environment, the other is which vendor answers
// which tier. Whoever binds first has to be able to see that the credential
// for it has arrived.
func TestAKeyIsSealedOnAnInstallationThatHasBoundNothing(t *testing.T) {
	e := SetupSearch(t)
	env := config.Static(map[string]string{
		keyvault.EnvRootKey: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32)),
		"GEMINI_API_KEY":    "a-gemini-key",
	})

	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, "", env, discard())
	if err != nil {
		t.Fatalf("ResolveRouting on an unbound installation: %v", err)
	}
	// Unchanged: nothing here binds anything. The lanes stay absent.
	if !cfg.Unconfigured() {
		t.Errorf("resolved %+v, want unconfigured", cfg.Tiers)
	}

	stored, err := settings.Get(e.adminRoutingCtx(), compose.NewSettingsStore(e.Pool), ai.ProviderKeys)
	if err != nil {
		t.Fatalf("reading the recorded refs: %v", err)
	}
	if stored["gemini"] == "" {
		t.Errorf("no ref recorded for gemini: %v — the key stayed in the environment, so Settings -> AI reports it unkeyed", stored)
	}
}

// A declared binding reaches an installation that was created without one.
//
// `seeds.ai_routing` used to be consumed only inside the creating transaction.
// That is every installation EXCEPT the ones that matter here: the desktop
// bundles ship a database, so their company was created on the build
// machine and the recipient's declaration was read by nothing. They set a
// provider key, and their AI surfaces answered from the offline fake with the
// one screen that looks like the fix naming the file that was already ignored.
//
// Insert-only, so the two halves of the claim are one test: it plants a binding
// where there is none, and it does not touch one that exists.
func TestADeclaredBindingReachesAnInstallationCreatedWithoutOne(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	// Precondition: nothing bound, which is the state a seeded bundle boots in.
	before, err := compose.ResolveRouting(ctx, e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting before the seed: %v", err)
	}
	if !before.Unconfigured() {
		t.Fatalf("fixture already bound %+v; this test needs an unbound installation", before.Tiers)
	}

	if err := compose.SeedRoutingIfUnset(ctx, e.Pool, routingSeedNode(t, offlineRouting), discard()); err != nil {
		t.Fatalf("planting the declared binding: %v", err)
	}

	after, err := compose.ResolveRouting(ctx, e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting after the seed: %v", err)
	}
	if after.Unconfigured() {
		t.Fatal("the declared binding did not reach the installation — its AI surfaces stay on the fake")
	}
	if got := after.Tiers["premium"].Model; got != "fake-large" {
		t.Errorf("premium bound to %q, want the declared fake-large", got)
	}
}

// The other half, and the reason this may run on EVERY boot of every role: a
// binding somebody chose is never reverted by a file. Without insert-only, an
// admin who re-points a lane in Settings -> AI would find the deployment's
// value back on the next restart, with nothing saying why.
func TestPlantingNeverOverwritesABindingTheInstallationAlreadyHas(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "chosen-by-a-human")); err != nil {
		t.Fatalf("storing the admin's binding: %v", err)
	}

	if err := compose.SeedRoutingIfUnset(ctx, e.Pool, routingSeedNode(t, offlineRouting), discard()); err != nil {
		t.Fatalf("SeedRoutingIfUnset over an existing binding: %v", err)
	}

	after, err := compose.ResolveRouting(ctx, e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if got := after.Tiers["premium"].Model; got != "chosen-by-a-human" {
		t.Errorf("premium is %q — the deployment's file overwrote a binding somebody chose", got)
	}
}

// A deployment that declares nothing is not an error and plants nothing: most
// installations declare no seed at all, and every one of them boots through
// this call.
func TestPlantingIsANoOpWhenTheDeploymentDeclaresNoBinding(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	if err := compose.SeedRoutingIfUnset(ctx, e.Pool, yaml.Node{}, discard()); err != nil {
		t.Fatalf("SeedRoutingIfUnset with nothing declared: %v", err)
	}
	after, err := compose.ResolveRouting(ctx, e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if !after.Unconfigured() {
		t.Errorf("an undeclared seed bound %+v", after.Tiers)
	}
}

// routingSeedNode parses a routing document into the yaml.Node the deployment
// config carries it as, so these tests exercise the same decode path a real
// margince.yaml takes rather than a struct built by hand.
func routingSeedNode(t *testing.T, body string) yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("parsing the seed fixture: %v", err)
	}
	// Unmarshal wraps the document in a DocumentNode; the config carries the
	// mapping itself.
	if doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 {
		return *doc.Content[0]
	}
	return doc
}

// An UNPROVISIONED installation is left alone. It has no workspace to attribute
// a settings write to, and the claim flow that creates one seeds the same value
// inside its own transaction — so planting here would either fail or write a
// row the bootstrap is about to write properly.
func TestPlantingLeavesAnUnprovisionedInstallationAlone(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()
	// Archiving every workspace is what "unprovisioned" IS to this path:
	// singletonWorkspace enumerates the LIVE ones. Same mechanism the test
	// above uses.
	if _, err := e.Owner.Exec(ctx,
		`UPDATE workspace SET archived_at = now() WHERE archived_at IS NULL`); err != nil {
		t.Fatalf("clearing the harness company: %v", err)
	}

	if err := compose.SeedRoutingIfUnset(ctx, e.Pool, routingSeedNode(t, offlineRouting), discard()); err != nil {
		t.Fatalf("planting on an unprovisioned installation failed the boot: %v", err)
	}

	var planted map[string]any
	if err := e.Owner.QueryRow(ctx,
		`SELECT value FROM setting WHERE key = $1`, ai.RoutingKey).Scan(&planted); err == nil {
		t.Errorf("a row was written for an installation with no workspace: %v", planted)
	}
}

// A MALFORMED seed is not this call's error to report. The bootstrap refuses it
// and fails the boot, loudly and once; repeating that on every later start
// would turn one actionable failure into a recurring one nobody can act on
// differently. So this plants nothing and lets the boot continue.
func TestPlantingIsSilentOnASeedTheBootstrapAlreadyRefuses(t *testing.T) {
	e := SetupSearch(t)
	ctx := context.Background()

	// A tier the task contract does not declare: refused by the same parser a
	// stored binding goes through, which is the point of decoding it there.
	bad := routingSeedNode(t, `profile: eu_hosted
tiers:
  not_a_tier: {provider: fake, model: fake-small}
embeddings: {provider: fake, model: fake-embed, dimensions: 8}
`)

	if err := compose.SeedRoutingIfUnset(ctx, e.Pool, bad, discard()); err != nil {
		t.Fatalf("a malformed seed failed the boot here instead of at the bootstrap: %v", err)
	}
	after, err := compose.ResolveRouting(ctx, e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if !after.Unconfigured() {
		t.Errorf("a refused seed still bound %+v", after.Tiers)
	}
}
