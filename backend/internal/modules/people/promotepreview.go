// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PreviewLeadPromotion answers what PromoteLead would do to this lead —
// merge into a person we already hold, or create one — by running the same
// PO-F-1 ladder promoteTarget runs, without writing (ADR-0119/A170).
//
// It returns a person, so it is a read and carries the row-scope gate: a
// matched person outside the caller's scope is withheld, and the response
// still says `merge`, because the outcome is a fact about the lead. A caller
// must never read an absent person as `create`.
func (s *Store) PreviewLeadPromotion(ctx context.Context, id ids.LeadID) (crmcontracts.PromoteLeadPreview, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return crmcontracts.PromoteLeadPreview{}, err
	}
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return crmcontracts.PromoteLeadPreview{}, err
	}
	out := crmcontracts.PromoteLeadPreview{Outcome: crmcontracts.PromoteLeadPreviewOutcomeCreate}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		// A decision read: the lead's core columns are what the ladder
		// consumes; custom columns play no part.
		lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		if err != nil {
			return fmt.Errorf("read lead before preview: %w", err)
		}
		if lead.Status == crmcontracts.LeadStatusPromoted {
			e := &AlreadyPromotedError{}
			if lead.PromotedPersonId != nil {
				e.PersonID = ids.From[ids.PersonKind](ids.UUID(*lead.PromotedPersonId))
			}
			return e
		}
		if lead.ArchivedAt != nil {
			// Disqualified: nothing promotion could do, so nothing to preview.
			return apperrors.ErrConflict
		}
		// The promotion refuses a lead nothing names, so the preview says so
		// too. Answering "will create" and then refusing the act is worse than
		// refusing outright, because the answer is what a human read and
		// agreed to before pressing the button.
		if leadIdentityName(lead) == "" {
			return &PromoteNeedsIdentityError{}
		}
		match, err := s.previewTarget(ctx, tx, lead)
		if err != nil {
			return err
		}
		if match.Decision != DecisionExactCollision {
			return nil
		}
		out.Outcome = crmcontracts.PromoteLeadPreviewOutcomeMerge
		// Two gates before a person is returned: the object grant (may this
		// role read people at all) and WRITE authority over the row — the
		// same probe promotion itself takes, because merging changes the
		// matched person. A readable person the caller may not change is
		// withheld here exactly as promotion will refuse it, so the preview
		// never promises a merge the act then declines.
		grantErr := auth.Require(ctx, "person", principal.ActionRead)
		if grantErr != nil && !errors.Is(grantErr, apperrors.ErrPermissionDenied) {
			return grantErr
		}
		writable, err := auth.WritableBy(ctx, tx, "person", match.PersonID.UUID)
		if err != nil {
			return err
		}
		if grantErr != nil || !writable {
			withheld := true
			out.PersonWithheld = &withheld
			return nil
		}
		person, err := readPerson(ctx, tx, match.PersonID, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read merge-target person: %w", err)
		}
		out.Person = &person
		return nil
	})
	return out, err
}

// previewTarget is the read half of promoteTarget: the same candidate the
// promotion would resolve, through the same ladder, so the preview and the
// promotion cannot disagree about who the lead already is.
//
// Held by: TestTheLadderCandidateIsDerivedInOnePlace (backend/internal/modules/people/onederivation_test.go)
func (s *Store) previewTarget(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead) (PersonResolution, error) {
	candidate, err := s.leadPersonCandidate(ctx, tx, lead)
	if err != nil {
		return PersonResolution{}, err
	}
	return DedupePerson(ctx, tx, candidate)
}

// leadIdentityName is what a lead is called, worked out once: its own name if
// it has one, otherwise the email address that is the only other thing naming
// it, and the empty string when it has neither.
//
// The promotion's identity guard and the ladder candidate both read it here.
// A guard that decides "this lead has an identity" one way while the candidate
// works the name out another way admits exactly the leads it meant to refuse:
// `FullName != nil` is true of a full_name that is PRESENT and EMPTY, which is
// not a name, and such a lead reaches the ladder carrying nothing to match on.
//
// The name is trimmed for the reason the address is (values.ParseEmail):
// padding is not identity, and a person stored under a padded name is a person
// the next search does not find.
func leadIdentityName(lead crmcontracts.Lead) string {
	if name := strings.TrimSpace(deref(lead.FullName)); name != "" {
		return name
	}
	if lead.Email != nil {
		return string(*lead.Email)
	}
	return ""
}

// leadPersonCandidate turns a lead into the candidate the §1.3 ladder matches
// on. The preview and the promotion take it from here rather than each
// assembling one, because "the same candidate" is a promise about the
// DERIVATION and not about the struct: two literals with the same fields still
// disagree when the names fed into them are worked out differently, and the
// preview would then name a person the promotion does not land on.
//
// A lead reaching here can carry a full_name that is present and empty, which
// is not the same as absent — the identity check upstream refuses only a lead
// with neither field, so an empty name with no email survives it. The email is
// therefore read through its own nil check rather than on the strength of that
// check having passed.
func (s *Store) leadPersonCandidate(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead) (PersonCandidate, error) {
	name := leadIdentityName(lead)
	var emails []string
	if lead.Email != nil {
		emails = []string{string(*lead.Email)}
	}
	consumerMail, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return PersonCandidate{}, err
	}
	return PersonCandidate{FullName: name, Emails: emails, ConsumerMail: consumerMail}, nil
}
