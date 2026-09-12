// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package contact360 assembles the contact record page in one round trip —
// the contact half of the one-composite-read doctrine (PO-EXT-3).
//
// It is the company 360's sibling and deliberately its mirror: one
// workspace transaction so the sections describe one moment, a mandatory
// root read whose refusal is the whole read's refusal, and every other
// section attempted independently and OMITTED-AND-NAMED when the caller
// lacks its grant. Empty and forbidden are different facts, and a page
// that renders them the same way tells the reader the relationship is
// cold when it is only invisible.
package contact360

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// sectionCap bounds every nested collection. These are summaries, not
// paging surfaces: page two comes from the endpoint that owns the
// collection, with its own cursor vocabulary.
const sectionCap = 25

// Service assembles the composite read from the module stores it composes.
type Service struct {
	pool     *pgxpool.Pool
	contacts *contacts.Store
	deals    *deals.Store
	projects *projects.Store
	consent  *consent.Store
	comms    *comms.Store
	// feedback is the correction ledger, consulted so a moment a human
	// dismissed does not come back.
	feedback *ai.FeedbackStore
	now      func() time.Time
	// providers is the adapter registry the provider section reads its
	// category vocabulary from. Nil where no adapter is compiled in.
	providers providerDescriptors
}

// NewService binds the composite read to its module stores. now is the
// injected clock — a test pins a fixed instant so a strength half-life
// cannot flake between seeding and reading.
func NewService(
	pool *pgxpool.Pool,
	contactsStore *contacts.Store,
	dealsStore *deals.Store,
	projectStore *projects.Store,
	consentStore *consent.Store,
	commsStore *comms.Store,
	feedbackStore *ai.FeedbackStore,
	now func() time.Time,
) *Service {
	return &Service{
		pool: pool, contacts: contactsStore, deals: dealsStore, projects: projectStore,
		consent: consentStore, comms: commsStore, feedback: feedbackStore, now: now,
	}
}

// WithProviders binds the licensed-data-provider registry, which the provider
// section reads the category vocabulary from — what a run did NOT ask for is
// the difference between the provider's full offering and what it requested,
// and only the descriptor knows the first half.
//
// Optional: a deployment with no adapter shows the section in its
// "not connected" state, and a nil registry there simply leaves the
// not-requested list empty rather than asserting a vocabulary nobody offers.
func (s *Service) WithProviders(reg providerDescriptors) *Service {
	s.providers = reg
	return s
}

// providerDescriptors is the slice of the adapter registry this package
// needs. Narrowed to one method so contact360 does not depend on the
// integrations module for a read that only wants a category list.
type providerDescriptors interface {
	Descriptor(name string) (provider.Descriptor, error)
}

// AssembleOptions narrows what the page reads. The zero value is the whole
// record.
type AssembleOptions struct {
	// ProjectID scopes the timeline sections — recent activity, next steps,
	// last touch, since-last-visit — to one body of work: rows filed under
	// this project or under none, never rows filed under another project.
	// The rule is activities.ActivityWithinProject; the identity, employment
	// and network sections describe the contact, not a project, and stay whole.
	ProjectID *ids.ProjectID
}

// Assemble reads the whole contact page inside ONE workspace transaction.
func (s *Service) Assemble(ctx context.Context, contactID ids.ContactID) (crmcontracts.Contact360, error) {
	return s.AssembleScoped(ctx, contactID, AssembleOptions{})
}

// AssembleScoped is Assemble narrowed by opts.
func (s *Service) AssembleScoped(ctx context.Context, contactID ids.ContactID, opts AssembleOptions) (crmcontracts.Contact360, error) {
	// The contact half of the same admission company360.AssembleScoped states, and
	// for the same reason: GetContactTx below asks it, and that read is still
	// what refuses an unreadable contact, but an admission living entirely in
	// another package is one refactor from living nowhere.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.Contact360{}, err
	}
	now := s.now().UTC()
	out := crmcontracts.Contact360{
		AsOf:            now,
		SectionsOmitted: []crmcontracts.Contact360SectionsOmitted{},
	}
	// The custom-field catalog is read above the transaction, not inside it:
	// it opens one of its own, and this page holds the only connection its
	// sections have for as long as it runs.
	active, err := s.contacts.ActiveContactColumns(ctx)
	if err != nil {
		return crmcontracts.Contact360{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		contact, err := s.contacts.GetContactTx(ctx, tx, contactID, storekit.LiveOnly, active)
		if err != nil {
			return err
		}
		out.Contact = contact
		// The scope is a read of the project, gated before any section
		// filters on it — and as the whole read's refusal, not a section's:
		// a page narrowed to a project the caller may not see has no honest
		// sections at all.
		if opts.ProjectID != nil {
			if err := activities.RequireProjectScope(ctx, tx, *opts.ProjectID); err != nil {
				return err
			}
			scope, err := activities.ReadProjectScope(ctx, tx, *opts.ProjectID, func(arg func(any) int) string {
				return fmt.Sprintf(contactReachesActivity, arg(contactID))
			})
			if err != nil {
				return err
			}
			wired := scope.Wire()
			out.Scope = &wired
		}

		for _, section := range s.sections(contactID, now, opts) {
			if err := section.read(ctx, tx, &out); err != nil {
				// A section the caller may not read is named, not returned
				// empty. Any other failure is the whole read's failure —
				// half a record page is worse than an error, because the
				// reader cannot tell which half is missing.
				if errors.Is(err, apperrors.ErrPermissionDenied) {
					out.SectionsOmitted = append(out.SectionsOmitted, section.name)
					continue
				}
				return fmt.Errorf("contact 360 section %q: %w", section.name, err)
			}
		}
		return nil
	})
	if err != nil {
		return crmcontracts.Contact360{}, err
	}
	return out, nil
}

// section is one independently-authorized part of the page.
type section struct {
	name crmcontracts.Contact360SectionsOmitted
	read func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error
}

func (s *Service) sections(contactID ids.ContactID, now time.Time, opts AssembleOptions) []section {
	return []section{
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedStrength, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.strengthSection(ctx, tx, contactID, now, opts.ProjectID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedRelationshipChanges, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.relationshipChangesSection(ctx, tx, contactID, now, opts.ProjectID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedEmployments, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.employmentsSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedDealRoles, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.dealRolesSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedProjects, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.projectsSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedActivities, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.activitiesSection(ctx, tx, contactID, opts, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextSteps, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.nextStepsSection(ctx, tx, contactID, opts, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedLastTouch, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.lastTouchSection(ctx, tx, contactID, opts, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNetwork, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.networkSection(ctx, tx, contactID, now, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedConsent, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.consentSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedDeadAddresses, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.deadAddressesSection(ctx, tx, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedProfileFields, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.profileFieldsSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedSinceLastVisit, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.sinceLastVisitSection(ctx, tx, contactID, opts, out)
		}},
		// Both of these run BEFORE the moments below, because the ladder's
		// rules read them: the meeting-prep rung asks what is booked, and the
		// missing-next-step rung asks whether an open deal has nothing
		// scheduled on it.
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedClaims, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.claimsSection(ctx, tx, contactID, opts, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedCommercial, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.commercialSection(ctx, tx, contactID, out)
		}},
		// What a licensed provider was PAID to tell us about this contact
		// (ADR-0101). Beside the canonical record, never folded into it.
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedProviderProfile, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.providerProfileSection(ctx, tx, contactID, out)
		}},
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedNextMeeting, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.nextMeetingSection(ctx, tx, contactID, now, opts, out)
		}},
		// LAST, and it has to be: the moments are derived from what the
		// sections above gathered, so a moment can never cite evidence this
		// page is not showing, and a section withheld for want of a grant
		// contributes no moments rather than leaking through one.
		{name: crmcontracts.Contact360SectionsOmittedContact360SectionsOmittedMoments, read: func(ctx context.Context, tx pgx.Tx, out *crmcontracts.Contact360) error {
			return s.momentsSection(ctx, tx, contactID, now, out)
		}},
	}
}

// requireRead is the object grant for a section. A denial returns the
// sentinel unchanged so Assemble can name the section rather than fail.
func requireRead(ctx context.Context, object string) error {
	return auth.Require(ctx, object, principal.ActionRead)
}

// edgeAlias is the alias every statement in this package gives the
// relationship table. Named rather than passed, because it is a property of how
// these statements are written and not a choice a caller makes — and a caller
// free to pass a different one could bound the wrong table.
const edgeAlias = "r"

// edgeScope resolves the relationship edge's read admission and its row bound
// together, answering the narrows-nothing predicate for an unbounded caller.
//
// The sections that read an edge already ask requireRead for it, so this is
// where the ENDPOINT CONJUNCTION arrives: an edge names two records and is
// bounded by both, where a single endpoint's scope clause bounds it by one. A
// denial still returns the sentinel unchanged, so a section reached without the
// grant is named rather than failed.
func edgeScope(ctx context.Context, arg func(any) int) (string, error) {
	clause, err := auth.EdgeReadScope(ctx, edgeAlias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return scopeAll, nil
	}
	return clause, nil
}
