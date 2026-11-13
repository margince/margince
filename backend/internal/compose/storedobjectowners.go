// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/storedobjectreap"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
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
func unreferencedKeysBy(db *database.DB) map[storedobjects.Kind]storedobjectreap.Owner {
	return map[storedobjects.Kind]storedobjectreap.Owner{
		storedobjects.KindRawOriginal: capture.NewSink(db).UnreferencedOriginalKeys,
		storedobjects.KindAttachment:  activities.NewStore(db).UnreferencedAttachmentKeys,
		storedobjects.KindKnowledge:   knowledge.NewStore(db).UnreferencedDocumentKeys,
		storedobjects.KindLogo: storedobjectreap.UnreferencedByAll(contacts.NewStore(db).UnreferencedLogoKeys,
			deals.NewStore(db, DealsInstallation()).UnreferencedIssuerLogoKeys),
		storedobjects.KindOffer:  deals.NewStore(db, DealsInstallation()).UnreferencedOfferKeys,
		storedobjects.KindImport: migration.NewRunStore(db).UnreferencedImportKeys,
		storedobjects.KindAvatar: identity.NewServiceFor(db).UnreferencedAvatarKeys,
	}
}

// adjudicate asks this installation's owners about the provisional keys.
func adjudicate(
	ctx context.Context, db *database.DB, provisional []storedobjects.Provisional,
) (storedobjectreap.Verdict, error) {
	return storedobjectreap.Adjudicate(ctx, unreferencedKeysBy(db), provisional)
}
