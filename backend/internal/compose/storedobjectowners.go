// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// unreferencedKeysBy answers, per kind, which of the keys its owning table does not
// carry. The seam is here because only compose may see every module at once: the
// ledger sits below all of them and no module may read another's rows.
//
// EVERY KIND HAS AN ENTRY. A kind nobody answers for is a key the sweep cannot
// adjudicate, and the only safe thing to do with it is nothing. A test holds this map
// to storedobjects.AllKinds() so a new kind cannot be declared without one, and
// Record refuses a kind outside that vocabulary, so reaching the sweep without an
// owner takes both a new kind and a missing entry.
func unreferencedKeysBy(db *database.DB) map[storedobjects.Kind]func(context.Context, []string) ([]string, error) {
	return map[storedobjects.Kind]func(context.Context, []string) ([]string, error){
		storedobjects.KindAttachment: activities.NewStore(db).UnreferencedAttachmentKeys,
		storedobjects.KindKnowledge:  knowledge.NewStore(db).UnreferencedDocumentKeys,
		storedobjects.KindLogo:       contacts.NewStore(db).UnreferencedLogoKeys,
		storedobjects.KindOffer:      deals.NewStore(db, DealsInstallation()).UnreferencedOfferKeys,
		storedobjects.KindImport:     migration.NewRunStore(db).UnreferencedImportKeys,
	}
}

// adjudicated is what one pass over the ledger established.
type adjudicated struct {
	// orphans are keys whose owning row never arrived: bytes to delete.
	orphans []string
	// adopted are keys an owning row DOES carry. Their bytes stay, and their intent
	// is retired: a referenced key still in the ledger is a clear that never ran,
	// and left there it occupies a slot in every later pass's limit — which is how a
	// handful of them starve every orphan behind them.
	adopted []string
}

// adjudicate groups what the ledger holds and asks each kind's owner.
//
// A kind with no owner registered is reported and the pass CONTINUES for every other
// kind: aborting would discard the orphans already adjudicated and leave every owned
// kind uncollected because one unowned kind exists. The fault is still returned, so
// it reaches an operator rather than passing as a clean sweep.
func adjudicate(
	ctx context.Context, db *database.DB, provisional []storedobjects.Provisional,
) (adjudicated, error) {
	byKind := map[storedobjects.Kind][]string{}
	for _, p := range provisional {
		byKind[p.Kind] = append(byKind[p.Kind], p.Key)
	}
	owners := unreferencedKeysBy(db)
	var out adjudicated
	var unowned []storedobjects.Kind
	for kind, keys := range byKind {
		owner, ok := owners[kind]
		if !ok {
			unowned = append(unowned, kind)
			continue
		}
		unreferenced, err := owner(ctx, keys)
		if err != nil {
			return out, err
		}
		out.orphans = append(out.orphans, unreferenced...)
		out.adopted = append(out.adopted, keysExcept(keys, unreferenced)...)
	}
	if len(unowned) > 0 {
		return out, fmt.Errorf("the stored-object sweep has no owner for kind(s) %v, so their keys cannot be adjudicated", unowned)
	}
	return out, nil
}

// keysExcept is the complement: everything the owner did not call unreferenced.
func keysExcept(keys, unreferenced []string) []string {
	gone := make(map[string]bool, len(unreferenced))
	for _, key := range unreferenced {
		gone[key] = true
	}
	var kept []string
	for _, key := range keys {
		if !gone[key] {
			kept = append(kept, key)
		}
	}
	return kept
}
