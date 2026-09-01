// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Where the model binding comes from.
//
// It is a `setting` row, not a file read at boot. A binding an operator could
// only change by editing a file on the server and restarting both roles is one
// they change rarely and out of band — and in between, the api and the worker
// can be running different bindings with nothing saying so. In the database
// there is one answer and every role that asks gets the same one.
//
// The routing FILE no longer seeds it. It used to, and that is exactly why it
// had to stop: `margince.yaml`'s `seeds.ai_routing` seeds the same setting, so
// two files could plant one installation's binding with nothing deciding
// between them and nothing saying which had. The boot file is the survivor
// because it reaches every path that creates an installation — the claim flow,
// the file bootstrap and a data reset all run the same seed inside the
// creating transaction — where the flag only ever fired on a boot that found
// the setting unset.
//
// A `--ai-routing` still on a command line is therefore ignored, loudly, and
// the warning names both replacements. The flag stays REGISTERED so an
// operator's existing command line does not die on an unknown flag; the file
// FORMAT stays too, because the debug lanes and the certification runner read
// it deliberately, with no database open.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// routingSeedActor names the boot in the audit trail when a stored binding is
// planted from the deployment's routing file. It is a system actor because no
// human asked: the value was already this installation's, in a file.
const routingSeedActor = "routing-seed"

// ResolveRouting settles the binding this process runs on: whatever the
// database holds, and nothing else. Every role that asks gets the same answer,
// which is the property a file read per-process cannot offer.
//
// A zero RoutingConfig means unconfigured, which is not an error: an
// installation that has bound no models runs with its AI lanes absent, exactly
// as one with no routing file did. It is reached from the UI without a restart,
// so an unset binding is a state somebody can see and fix rather than a boot
// this function has to guess its way out of.
func ResolveRouting(ctx context.Context, pool *pgxpool.Pool, routingPath string, keys config.Lookup, log *slog.Logger) (ai.RoutingConfig, error) {
	ws, err := singletonWorkspace(ctx, pool)
	if err != nil {
		return ai.RoutingConfig{}, err
	}
	if ws == (ids.UUID{}) {
		// Unprovisioned (ADR-0105): the claim flow has not run, so there is no
		// installation to hold a binding. Serving is the point — every tenant
		// route answers 503 until it is claimed — so this is not a boot error.
		//
		// The ignored flag is still announced here, before the return. The flag
		// help promises a warning whenever the path is ignored, and an
		// unprovisioned boot is the one case that reached no warning at all —
		// which is also the boot an operator is most likely to be watching,
		// having just started a fresh installation with the flag they have
		// always used.
		if routingPath != "" {
			log.WarnContext(ctx,
				"--ai-routing is ignored: this installation is not provisioned yet, so nothing holds a model binding. Declare one under `seeds.ai_routing` in margince.yaml before the claim flow runs, or set it under Settings -> AI once the installation is claimed. Remove the flag; the file is read only by the debug and certification lanes now",
				"file", routingPath)
		}
		return ai.RoutingConfig{}, nil
	}
	ctx = routingCtx(ctx, ws)

	stored, err := readStoredRouting(ctx, pool)
	if err != nil {
		return ai.RoutingConfig{}, err
	}
	if routingPath != "" {
		warnRoutingFileIgnored(ctx, routingPath, stored.Unconfigured(), log)
	}
	if stored.Unconfigured() {
		return ai.RoutingConfig{}, nil
	}
	lookup, credentials := sealedKeys(ctx, pool, ws, keys, log)
	cfg, err := ai.FromStored(stored, lookup)
	if err != nil {
		return ai.RoutingConfig{}, err
	}
	return cfg.WithCredentialVersion(credentials), nil
}

// warnRoutingFileIgnored says a `--ai-routing` was passed and did nothing.
//
// Two different situations, and an operator needs to be told which one they are
// in. With a binding already stored the file is merely redundant — the flag can
// come off the command line and nothing changes. With NO binding stored the
// flag used to be what supplied one, so this boot is the one where the AI lanes
// go absent, and saying only "ignored" would leave the operator to discover
// that from a feature that stopped working.
func warnRoutingFileIgnored(ctx context.Context, routingPath string, unconfigured bool, log *slog.Logger) {
	if unconfigured {
		// Says "and restart" deliberately. A process that boots with nothing
		// bound wires no model path, and the watcher that would notice a later
		// write is built from that path — so a binding set through the UI now is
		// stored correctly and picked up on the next start, not on this one.
		// Promising an immediate effect here would send an operator looking for
		// a lane that was never going to light up.
		log.WarnContext(ctx,
			"--ai-routing is ignored and this installation has NO stored model binding, so its AI lanes are absent for the life of this process: declare the binding under `seeds.ai_routing` in margince.yaml before a fresh install's first boot, or set it through Settings -> AI and restart this role to pick it up",
			"file", routingPath)
		return
	}
	log.WarnContext(ctx,
		"--ai-routing is ignored: this installation's model binding is a stored setting, changed through Settings -> AI or PUT /v1/ai/routing. Remove the flag; the file is read only by the debug and certification lanes now",
		"file", routingPath)
}

// sealedKeys upgrades the environment lookup to one that resolves a provider's
// BYOK key from the vault, sealing any the environment still carries.
//
// Built here rather than passed in because this is where the workspace is
// already resolved, and both roles would otherwise construct the same thing
// from the same three inputs. A second vault instance costs nothing: it holds a
// pool and an AEAD derived from the root key, and no state that two of them
// could disagree about.
//
// An installation with no vault configured gets the environment unchanged,
// which is where every installation was before this existed.
func sealedKeys(ctx context.Context, pool *pgxpool.Pool, ws ids.UUID, env config.Lookup, log *slog.Logger) (config.Lookup, string) {
	vault, configured, err := keyvault.FromEnv(ctx, pool, env)
	if err != nil || !configured {
		if err != nil {
			// Not fatal HERE, deliberately. FromEnv refuses a boot that finds
			// sealed ciphertext with no root key, so by the time this runs the
			// serving roles have already had their say on whether the vault is
			// survivable. What is left for this path is a vault that cannot be
			// built at all, and provider keys still resolve from the
			// environment — which is where every installation kept them before
			// the vault existed.
			log.WarnContext(ctx, "the key vault is configured but unusable; provider credentials resolve from the environment this boot", "error", err)
		}
		return env, ""
	}
	workspace := ids.From[ids.WorkspaceKind](ws)
	refs := SealProviderKeys(ctx, pool, vault, workspace, env, log)
	return ai.SealedKeys(ctx, vault, workspace, refs, env), credentialDigest(refs)
}

// credentialDigest fingerprints the provider→ref map a binding resolves with, so
// a rotated or removed key is observable to a watcher that would otherwise see
// an identical routing digest and keep serving the old credential.
//
// Over the REFS, never the secrets: a ref is an opaque handle, and the digest of
// one tells a reader only whether it moved. Sorted, because a map's iteration
// order is not stable and an unsorted digest would report a change on every
// tick.
//
// The empty map digests to the empty string rather than to a hash of nothing, so
// an installation with no vault and one with a vault holding no keys compare
// equal — neither has a credential to fall behind on.
func credentialDigest(refs map[string]string) string {
	if len(refs) == 0 {
		return ""
	}
	providers := make([]string, 0, len(refs))
	for provider := range refs {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	// Rendered to one string and hashed once, rather than written into the hash
	// incrementally: a hash.Hash's Write never fails, but a call whose error is
	// ignored reads exactly like one that should have been handled.
	var canonical strings.Builder
	for _, provider := range providers {
		// Length-prefixed so no two different maps can render the same bytes —
		// {"ab":"c"} and {"a":"bc"} concatenate identically without it.
		canonical.WriteString(strconv.Itoa(len(provider)))
		canonical.WriteString(":" + provider + "=")
		canonical.WriteString(strconv.Itoa(len(refs[provider])))
		canonical.WriteString(":" + refs[provider] + ";")
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(sum[:])
}

// routingCtx binds the boot's system principal on the installation's
// workspace. A PrincipalSystem passes the object gate unconditionally, which is
// how the worker's sweeps read their settings too — the alternative would be an
// ungated read, and `setting` has no RLS beneath that gate.
func routingCtx(ctx context.Context, ws ids.UUID) context.Context {
	return bootCtx(ctx, ws, routingSeedActor)
}

// bootCtx is that same binding for any boot-time settings read: the workspace,
// a named system actor, and a correlation id, so a settings write made before
// any request exists is still attributable to the thing that made it.
func bootCtx(ctx context.Context, ws ids.UUID, actor string) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: actor,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

func readStoredRouting(ctx context.Context, pool *pgxpool.Pool) (ai.RoutingConfig, error) {
	return settings.Get(ctx, NewSettingsStore(pool), ai.Routing)
}

// singletonWorkspace resolves the one live workspace this installation is
// (A107/ADR-0061), or the zero id when it has not been provisioned yet.
//
// A multi-workspace database is refused at boot by EnsureInstallation, which
// runs before this; taking the first here would be picking one of two answers
// to a question that must not have two.
func singletonWorkspace(ctx context.Context, pool *pgxpool.Pool) (ids.UUID, error) {
	live, err := enumerateWorkspaces(ctx, pool)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: resolving the installation's workspace: %w", err)
	}
	if len(live) == 0 {
		return ids.UUID{}, nil
	}
	return live[0], nil
}

// RoutingFromDeployConfig reads a deployment config file's declared tier→model
// binding — `seeds.ai_routing` — and returns it as a validated RoutingConfig.
//
// It exists for the certification lane, which needs to certify the models a
// deployment ACTUALLY binds rather than one an engineer typed. Nothing at
// runtime reads a binding this way: a running installation's binding is a stored
// setting (see this file's own note above), and `seeds.ai_routing` only seeds a
// fresh install. So this is a read of what a fresh install WOULD get, which is
// exactly the question `make e2e-ai ROUTING=` asks.
//
// The decode is routingSeedFrom's, not a second parser: an operator's file must
// mean the same thing to the cert lane as it does to bootstrap, including the
// alias and merge-key idioms routingSeedFrom resolves.
//
// A file that declares no binding is an error here, unlike at bootstrap where it
// is the ordinary case. Bootstrap has a default to fall back on — bind nothing,
// run the CRUD — and a certification run has none: there is no model to measure,
// and returning an empty config would spend the run's setup before failing on
// the first call.
func RoutingFromDeployConfig(path string, env runtimeenv.Environment) (ai.RoutingConfig, error) {
	// Stat first. deployconfig.Load TOLERATES an absent layer — that is right for
	// an optional overlay and wrong here, where the path is the whole instruction:
	// a typo would otherwise be reported as "declares no seeds.ai_routing", which
	// sends an operator to edit a file that does not exist.
	if _, err := os.Stat(path); err != nil {
		return ai.RoutingConfig{}, fmt.Errorf("compose: reading the deployment config %s: %w", path, err)
	}
	cfg, err := deployconfig.Load(path, env)
	if err != nil {
		return ai.RoutingConfig{}, fmt.Errorf("compose: reading the deployment config %s: %w", path, err)
	}
	routing, declared, err := routingSeedFrom(cfg.Seeds.AIRouting)
	if err != nil {
		return ai.RoutingConfig{}, err
	}
	if !declared {
		return ai.RoutingConfig{}, fmt.Errorf("compose: %s declares no seeds.ai_routing, so it names no model to certify — point ROUTING= at a config that binds one, or name a model outright with MODEL=", path)
	}
	return routing, nil
}
