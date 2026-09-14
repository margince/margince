// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The advice as a seam: what the rules raise for one caller, what that
// caller has dismissed, and the fingerprint both writers of advice share.
//
// The account scan merges the model's findings with the rules' own rows, and
// a dismissal has to hold across both. That works only if there is ONE
// definition of "what the rules raise" and ONE fingerprint — so the scan
// reaches these through the composite's own service rather than re-deriving
// either, and the dismissal endpoint asks the scan, through the recogniser
// below, whether a fingerprint it does not recognise is one of the scan's.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SuggestionFingerprint identifies a suggestion by what it fired ON — the
// kind, its subject, and the records it cites — for every writer of advice
// on an account, so a dismissal keyed on it holds whichever writer raised
// the row.
func SuggestionFingerprint(kind, subject string, evidence []crmcontracts.CompanyBriefEvidence) string {
	return fingerprint(kind, subject, evidence)
}

// ScanRecogniser answers whether a stored account scan raises this
// fingerprint for the calling user. Set by compose when a scan surface
// exists; nil means no scan writes advice on this role.
type ScanRecogniser interface {
	RaisesForCaller(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, fingerprint string) (bool, error)
}

// RecogniseScanFindings lets the dismissal endpoint accept a fingerprint the
// account scan raised, so a reader can put a model's finding off exactly as
// they put a rule's off.
func (s *Service) RecogniseScanFindings(r ScanRecogniser) { s.scans = r }

// UndismissedAdvice lists what the rules raise on this account for this
// caller, minus what they have dismissed, with no display cap: the caller
// applies its own once it has merged what else it holds.
//
// Empty, not refused, for a caller who may read neither the timeline nor the
// pipeline — the composite omits the section for them, and the merged list
// simply has no rule rows in it.
func (s *Service) UndismissedAdvice(
	ctx context.Context, companyID ids.CompanyID,
) ([]crmcontracts.Company360Suggestion, error) {
	if _, err := actingUser(ctx); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	var kept []crmcontracts.Company360Suggestion
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		in, err := s.suggestionInputsFor(ctx, tx, companyID, now)
		if err != nil {
			return err
		}
		if !in.advisable() {
			return nil
		}
		found := candidateSuggestions(companyID, now, in)
		kept, err = s.withoutDismissed(ctx, tx, companyID, found)
		return err
	})
	return kept, err
}

// KeepUndismissed drops from found whatever this caller has already judged,
// for advice another writer raised. The fingerprints asked about are the ones
// in hand, never the caller's whole history.
//
// The SAME account gate UndismissedAdvice above opens with, for the same
// reason. Both read one ledger, both key it on a company id the caller
// supplies, and both answer whether this reader has judged advice about that
// account — so an account the caller cannot see must answer not-found to both.
// This one is reached only behind two of those checks today, which is why the
// gap was invisible: the protection was the call site's rather than the read's,
// and a second call site would not have inherited it.
func (s *Service) KeepUndismissed(
	ctx context.Context, companyID ids.CompanyID, found []crmcontracts.Company360Suggestion,
) ([]crmcontracts.Company360Suggestion, error) {
	if _, err := actingUser(ctx); err != nil {
		return nil, err
	}
	var kept []crmcontracts.Company360Suggestion
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "company", companyID.UUID); err != nil {
			return err
		}
		var err error
		kept, err = s.withoutDismissed(ctx, tx, companyID, found)
		return err
	})
	return kept, err
}

// suggestionInputsFor gathers the rules' inputs outside an assembly, the way
// the dismissal path does: the signal reading, the account's own heading, the
// installation's base currency, then the grant-gated reads.
func (s *Service) suggestionInputsFor(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time,
) (suggestionInputs, error) {
	facts, err := readSignalFacts(ctx, tx, companyID)
	if err != nil {
		return suggestionInputs{}, err
	}
	heading, err := readCompanyHeading(ctx, tx, companyID)
	if err != nil {
		return suggestionInputs{}, err
	}
	base, err := identity.BaseCurrencyOf(ctx, tx)
	if err != nil {
		return suggestionInputs{}, err
	}
	// The dismissal path reads the whole account: it answers "which suggestion
	// is this", not "what should this page advise", so there is no project to
	// narrow to.
	return gatherSuggestionInputs(ctx, tx, companyID, now, facts, heading, base, AssembleOptions{})
}
