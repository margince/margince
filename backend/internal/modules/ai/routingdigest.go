// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// bindingDigest is the routing half of the ai_call_config dimension key: a
// digest of what this config BINDS, not of the bytes it arrived in.
//
// It is taken after defaulting and validation, over the canonical JSON
// encoding, which makes two configs hash alike exactly when they route alike.
// json.Marshal orders struct fields by declaration and sorts map keys, so the
// encoding is deterministic across processes — the property this digest has to
// have to be compared at all.
//
// The distinction is not academic. This value reaches contactbrief.Fingerprint
// and its siblings, where it decides whether a stored brief may be reused; a
// digest of raw bytes meant that ADDING A COMMENT to the routing file
// invalidated every cached brief, dossier and growth-fit in the installation
// and regenerated them through paid models. Reindenting did it too, and so did
// writing `dimensions: 1536` where the default was already 1536.
//
// What must still change the digest is any change to the binding itself — a
// re-pointed tier, a different base URL, a narrowed `input`. Those are exactly
// the cases where content a model wrote must stop being attributed to a model
// that no longer produces it.
//
// It reads the RESOLVED lanes and leaves out Providers: the lanes already carry
// every host and pin, and digesting them twice would re-attribute every cached
// brief the day the document's shape changed while nothing it routes did.
//
// An embeddings host spelled as its provider's compiled default digests as the
// empty one it dials identically: the lift writes the default out to keep that
// lane off a gateway its tiers moved the provider to, and the binding is unchanged.
func (cfg RoutingConfig) bindingDigest() string {
	cfg.Providers = nil
	if cfg.Embeddings.BaseURL != "" && sameHost(cfg.Embeddings.Provider, cfg.Embeddings.BaseURL, "") {
		cfg.Embeddings.BaseURL = ""
	}
	return digestJSON(cfg)
}

// digestJSON is the sha256 of a routing document's JSON encoding, which orders
// struct fields by declaration and sorts map keys, so it is deterministic across
// processes.
func digestJSON(cfg RoutingConfig) string {
	// A plain struct of strings, ints and string-keyed maps — marshal cannot
	// fail on it, and the same spelling guards the sibling fingerprints in
	// compose/companybrief and compose/companydossier that this digest feeds.
	encoded, _ := json.Marshal(cfg) //nolint:errchkjson // plain scalars and string-keyed maps; marshal cannot fail
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
