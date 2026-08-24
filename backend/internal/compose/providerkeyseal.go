// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Moving a BYOK key out of the process environment and into the vault, once.
//
// An installation exporting GEMINI_API_KEY today needs to do nothing: the key
// is sealed on the next boot and the variable becomes how the key ARRIVED
// rather than where it lives. That is the same shape the routing file took when
// routing moved into the database, and for the same reason — a migration an
// operator has to perform is a migration half of them will not.
//
// What it does NOT do is remove the variable from their deployment. Nothing
// here can: the process cannot edit its own orchestrator. So the variable stays
// readable until an operator drops it, and the sentence this logs is what tells
// them they can.

import (
	"context"
	"log/slog"
	"maps"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/ai"
	"github.com/gradionhq/margince/backend/internal/platform/config"
	"github.com/gradionhq/margince/backend/internal/platform/keyvault"
	"github.com/gradionhq/margince/backend/internal/platform/settings"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// SealProviderKeys stores any BYOK key this installation supplies in the
// environment but has not sealed yet, and returns the refs to resolve with.
//
// Returns the refs it knows about even when sealing something fails, so a
// vault that refuses one provider does not cost the installation the keys it
// already sealed for the others.
func SealProviderKeys(ctx context.Context, pool *pgxpool.Pool, vault keyvault.Vault, ws ids.WorkspaceID, env config.Lookup, log *slog.Logger) map[string]string {
	store := NewSettingsStore(pool)
	stored, err := settings.Get(ctx, store, ai.ProviderKeys)
	if err != nil {
		log.WarnContext(ctx, "cannot read the sealed provider credentials; falling back to the environment for this boot", "error", err)
		return nil
	}
	if vault == nil {
		// No vault configured. The environment is still the source, which is
		// exactly where every installation was before this existed.
		return stored
	}

	next := maps.Clone(stored)
	if next == nil {
		next = map[string]string{}
	}
	sealedNow := make([]string, 0, len(ai.CloudProvidersNeedingKeys()))
	for _, provider := range ai.CloudProvidersNeedingKeys() {
		if _, already := next[provider]; already {
			continue
		}
		secret := env(ai.KeyEnvVarFor(provider))
		if secret == "" {
			continue
		}
		ref, err := vault.Put(ctx, ws, []byte(secret))
		if err != nil {
			// One provider's failure is not the others'. The environment still
			// answers for this one, so the installation keeps working.
			log.ErrorContext(ctx, "cannot seal a provider credential; it stays in the environment for now",
				"provider", provider, "error", err)
			continue
		}
		next[provider] = string(ref)
		sealedNow = append(sealedNow, provider)
	}
	if len(sealedNow) == 0 {
		return stored
	}

	// Set, not SeedValue. Seeding is insert-only by design — it exists so a
	// restart never overwrites a value a human changed — and that is exactly
	// wrong here: the refs accumulate. An installation that sealed gemini last
	// boot and adds anthropic this one has a row already, so a seed would store
	// nothing, the new ref would never be recorded, and its blob would be
	// stranded in the vault while the environment silently kept answering.
	//
	// The merge is re-done INSIDE a locked transaction rather than trusting the
	// read above. "Overwriting is safe because this map is only ever grown" was
	// true while this was the only writer; it stopped being true when an admin
	// got a write path. Two ways it breaks, and the second is the worse one:
	//
	//   lost update       this reads {}, an admin's PUT commits, this writes
	//                     {openai: ref} and the admin's provider is dropped —
	//                     its blob stranded, and the admin was told 204
	//   resurrected ref   this reads {gemini: refA}, an admin's DELETE commits
	//                     and destroys refA's blob, this writes {gemini: refA}
	//                     back — the setting now names a blob that is gone,
	//                     which is the dangling ref the store's own error
	//                     classification goes to some length to avoid
	//
	// And the window is not boot-only: RoutingWatcher.Recheck reaches here every
	// interval in every role, so any installation still carrying an unsealed
	// *_API_KEY re-runs this write on a timer.
	//
	// Not literally sharing ai.ProviderKeyStore's swapRef: that moves ONE
	// provider and this merges many. What they must share is the lock, and they
	// do — settings.LockForWrite, taken before the read on both sides.
	merged := next
	if err := store.WriteTx(ctx, func(tx pgx.Tx) error {
		if err := settings.LockForWrite(ctx, tx, ai.ProviderKeysKey); err != nil {
			return err
		}
		current, err := settings.GetTx(ctx, tx, ai.ProviderKeys)
		if err != nil {
			return err
		}
		// The freshly sealed refs go on top of what is stored NOW. A provider
		// somebody else keyed while this was sealing keeps their ref: only the
		// providers this call actually sealed are written.
		merged = maps.Clone(current)
		if merged == nil {
			merged = map[string]string{}
		}
		for _, provider := range sealedNow {
			merged[provider] = next[provider]
		}
		return settings.SetTx(ctx, store, tx, ai.ProviderKeys, merged)
	}); err != nil {
		// The blobs are sealed and nothing references them — inert, encrypted
		// at rest, and collected by nobody. The installation keeps running on
		// the environment and the next boot tries again, which will strand
		// another. Loud because a stranded secret is not a non-event.
		log.ErrorContext(ctx, "sealed provider credentials could not be recorded; they are stranded in the vault and the environment is still the source",
			"providers", sealedNow, "error", err)
		return stored
	}
	next = merged
	// The one sentence that tells an operator they may now drop the variables.
	// Nothing here can remove them: the process cannot edit its orchestrator.
	log.InfoContext(ctx, "sealed provider credentials into the key vault; the environment variables that carried them can be removed",
		"providers", sealedNow)
	return next
}
