// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// SharedMergeReviews lets a live full-seat owner of one member of a contact
// case merge the members their colleagues own. Off by default, because it
// widens who may change a colleague's contact.
var SharedMergeReviews = config.DefineStored(config.Spec[bool]{
	Key:     "contacts.shared_merge_reviews",
	Env:     "MARGINCE_CONTACTS_SHARED_MERGE_REVIEWS",
	Type:    config.Bool,
	Default: config.Some(false),
	Doc: "Let any owner of a possible duplicate merge the whole case, keeping the other owners' access " +
		"to the remaining contact through a record grant.",
}, config.Store{
	RBACObject: "installation_settings", AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the case merge reads it for whoever merges
})

// sharedMergeReviewsForDecision reads the switch for a merge about to act on
// it. The shared lock makes a change of the switch wait for this transaction.
func (s *Store) sharedMergeReviewsForDecision(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := settings.ShareLockForDecision(ctx, tx, SharedMergeReviews.Key()); err != nil {
		return false, err
	}
	return config.StoredApplyTx(ctx, tx, s.bootConfig(), SharedMergeReviews)
}

func (s *Store) sharedMergeReviews(ctx context.Context) (bool, error) {
	if s.settings == nil {
		return false, errors.New("contacts: the case merge setting is not wired; the installation cannot read it")
	}
	return config.StoredVal(ctx, s.settings, SharedMergeReviews)
}

// SetSharedMergeReviews switches the policy. An admin's act alone: it decides
// who may change a colleague's contact.
func (s *Store) SetSharedMergeReviews(ctx context.Context, on bool) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	if s.settings == nil {
		return errors.New("contacts: the case merge setting is not wired; the installation cannot change it")
	}
	return config.Save(ctx, s.settings, SharedMergeReviews, on)
}

// GetContactCaseSettings implements GET /dedupe/settings.
func (h Handlers) GetContactCaseSettings(w http.ResponseWriter, r *http.Request) {
	on, err := h.store.sharedMergeReviews(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ContactCaseSettings{Enabled: on, Locks: h.settingLocks(SharedMergeReviews)})
}

// UpdateContactCaseSettings implements PUT /dedupe/settings (admin, human only).
func (h Handlers) UpdateContactCaseSettings(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.ContactCaseSettings
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := h.store.SetSharedMergeReviews(r.Context(), req.Enabled); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	req.Locks = h.settingLocks(SharedMergeReviews)
	httperr.WriteJSON(w, http.StatusOK, req)
}
