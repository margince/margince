// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The section names, spelled once. They are the contract's
// sections_omitted vocabulary and the keys the assembly reasons about, so
// a rename cannot leave the two halves disagreeing.
const (
	sectionContacts       = crmcontracts.Company360SectionsOmitted("contacts")
	sectionDeals          = crmcontracts.Company360SectionsOmitted("deals")
	sectionProjects       = crmcontracts.Company360SectionsOmitted("projects")
	sectionStrength       = crmcontracts.Company360SectionsOmitted("strength")
	sectionActivities     = crmcontracts.Company360SectionsOmitted("activities")
	sectionLastTouch      = crmcontracts.Company360SectionsOmitted("last_touch")
	sectionStateStrip     = crmcontracts.Company360SectionsOmitted("state_strip")
	sectionHealth         = crmcontracts.Company360SectionsOmitted("health")
	sectionTags           = crmcontracts.Company360SectionsOmitted("tags")
	sectionApprovals      = crmcontracts.Company360SectionsOmitted("pending_approvals")
	sectionNextSteps      = crmcontracts.Company360SectionsOmitted("next_steps")
	sectionSinceLastVisit = crmcontracts.Company360SectionsOmitted("since_last_visit")
	sectionSuggestions    = crmcontracts.Company360SectionsOmitted("suggestions")
	sectionNextMeeting    = crmcontracts.Company360SectionsOmitted("next_meeting")
	sectionBilling        = crmcontracts.Company360SectionsOmitted("billing_contacts")
)

// Service assembles the 360 and maintains the visit baseline.
type Service struct {
	pool      *pgxpool.Pool
	contacts  *contacts.Store
	deals     *deals.Store
	projects  *projects.Store
	approvals *approvals.Service
	now       func() time.Time
	// scans recognises a dismissal of advice the account scan raised, when a
	// deployment has one. See RecogniseScanFindings.
	scans ScanRecogniser
}

// NewService binds the composite read to the module stores it composes.
// now is the read's injected clock (the house shape: a test pins a fixed
// instant so a strength half-life or a stall window cannot flake between
// seeding and reading).
func NewService(
	pool *pgxpool.Pool,
	contactsStore *contacts.Store,
	dealsStore *deals.Store,
	projectStore *projects.Store,
	approvalsSvc *approvals.Service,
	now func() time.Time,
) *Service {
	return &Service{
		pool: pool, contacts: contactsStore, deals: dealsStore, projects: projectStore,
		approvals: approvalsSvc, now: now,
	}
}

// Assemble reads the whole company page inside ONE workspace transaction.
// The company read is mandatory and its refusal is the whole read's
// refusal; every other section is attempted, and a section refused for
// lack of a grant is omitted and named rather than returned empty.
func (s *Service) Assemble(ctx context.Context, companyID ids.CompanyID) (crmcontracts.Company360, error) {
	return s.AssembleScoped(ctx, companyID, AssembleOptions{})
}

// AssembleOptions narrows what the page reads. The zero value is the whole
// record.
type AssembleOptions struct {
	// ProjectID scopes the timeline and the last-touch dates to one body of
	// work: rows filed under this project or under none, never rows filed
	// under another project. The rule is activities.ActivityWithinProject;
	// contacts, deals and tags describe the account, not a project, and stay
	// whole.
	ProjectID *ids.ProjectID
}

// AssembleScoped is Assemble narrowed by opts.
func (s *Service) AssembleScoped(ctx context.Context, companyID ids.CompanyID, opts AssembleOptions) (crmcontracts.Company360, error) {
	// The object question, asked HERE rather than only by the anchor read
	// below. GetCompanyTx does ask it, and this page's refusal is still
	// its refusal — but a composite read whose authorization is entirely
	// somebody else's is one refactor away from having none, and nothing
	// would fail. Stated at the entry point, it is readable where the page is
	// entered and it survives the anchor moving.
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.Company360{}, err
	}
	now := s.now().UTC()
	out := crmcontracts.Company360{AsOf: now, SectionsOmitted: []crmcontracts.Company360SectionsOmitted{}}
	// The custom-field catalog is read above the transaction, not inside it:
	// it opens one of its own, and this page holds the only connection its
	// sections have for as long as it runs.
	active, err := s.contacts.ActiveCompanyColumns(ctx)
	if err != nil {
		return crmcontracts.Company360{}, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		company, err := s.contacts.GetCompanyTx(ctx, tx, companyID, storekit.LiveOnly, active)
		if err != nil {
			return err
		}
		out.Company = company
		if err := opts.admit(ctx, tx, companyID, &out); err != nil {
			return err
		}
		return s.sections(ctx, tx, companyID, now, opts, &out)
	})
	if err != nil {
		return crmcontracts.Company360{}, err
	}
	return out, nil
}

// sections runs each optional section behind its own grant, in a fixed
// order so two reads of the same account produce the same
// sections_omitted list. A section that refuses with
// apperrors.ErrPermissionDenied is omitted and named; any other error
// fails the whole read, because a section that broke for a real reason
// must never be reported as one the caller may not see.
func (s *Service) sections(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, opts AssembleOptions, out *crmcontracts.Company360) error {
	a := &assembly{svc: s, ctx: ctx, tx: tx, companyID: companyID, now: now, opts: opts, out: out}
	each := []struct {
		name crmcontracts.Company360SectionsOmitted
		read func() error
	}{
		{sectionContacts, a.readContacts},
		{sectionStrength, a.readStrength},
		{sectionDeals, a.readDeals},
		{sectionProjects, a.readProjects},
		{sectionActivities, a.readTimeline},
		{sectionLastTouch, a.readLastTouch},
		{sectionStateStrip, a.readStateStrip},
		{sectionHealth, a.readHealth},
		{sectionNextSteps, a.readNextSteps},
		{sectionNextMeeting, a.readNextMeeting},
		{sectionBilling, a.readBillingContacts},
		{sectionTags, a.readTags},
		{sectionApprovals, a.readPendingApprovals},
		{sectionSinceLastVisit, a.readSinceLastVisit},
		// Last because it is derived, not because it reads the sections above —
		// the rules issue their own queries (suggestionreads.go), under the same
		// grants and the same row scope, so they can see further back than a
		// truncated section page without ever seeing wider.
		{sectionSuggestions, a.readSuggestions},
		// After next_steps, whose rows it ranks. See moment.go.
		{sectionMoments, a.readMoment},
	}
	for _, section := range each {
		err := section.read()
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			out.SectionsOmitted = append(out.SectionsOmitted, section.name)
			continue
		}
		if err != nil {
			return err
		}
	}
	return a.decorate()
}

// assembly is one 360's working state. Several sections are built from the
// same underlying read — the contact list and the account roll-up both need
// every contact's score, and the approvals section and the since-last-visit
// count both need the decidable stagings — so those reads are taken once
// here and shared, rather than each section paying for its own copy of the
// same rows at the same instant.
type assembly struct {
	svc       *Service
	ctx       context.Context
	tx        pgx.Tx
	companyID ids.CompanyID
	now       time.Time
	opts      AssembleOptions
	out       *crmcontracts.Company360

	// baseCurrency is the installation's reporting currency, resolved at most
	// ONCE per assembly. It is one installation-wide value, and every section
	// that labels money wants the same answer — reading it per section costs a
	// query each and lets two sections disagree if the value moved between
	// them (the 360's query budget is what noticed).
	baseCurrency     string
	baseCurrencyRead bool

	contacts      []contacts.ContactStrength
	contactsRead  bool
	advice        suggestionInputs
	adviceRead    bool
	adviceErr     error
	signals       signalFacts
	signalsRead   bool
	signalsErr    error
	staged        []crmcontracts.Approval
	stagedRead    bool
	stagedRefused bool
}

// contactStrengths reads every visible contact's §4 score once per request.
func (a *assembly) contactStrengths() ([]contacts.ContactStrength, error) {
	if a.contactsRead {
		return a.contacts, nil
	}
	// Narrowed with the rest of the page. The health block reads its
	// active-contact count, its reply balance and its single-threaded flag off
	// these strengths, so an unscoped read put a number computed across every
	// project under a timeline showing one.
	contacts, err := contacts.StrengthForCompanyContacts(a.ctx, a.tx, a.companyID, a.now, a.opts.ProjectID)
	if err != nil {
		return nil, err
	}
	a.contacts, a.contactsRead = contacts, true
	return contacts, nil
}

// pendingApprovals reads the decidable stagings once per request.
// stagedRefused records a permission refusal so the count half can answer
// "not counted" without asking again and getting the same refusal.
func (a *assembly) pendingApprovals() ([]crmcontracts.Approval, bool, error) {
	if a.stagedRead {
		return a.staged, !a.stagedRefused, nil
	}
	staged, err := a.svc.approvals.PendingForTarget(a.ctx, a.tx, entityTypeCompany, a.companyID.UUID, approvals.PendingScanCap)
	a.stagedRead = true
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		a.stagedRefused = true
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	a.staged = staged
	return staged, true, nil
}

func (a *assembly) readContacts() error {
	if err := auth.Require(a.ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	strengths, err := a.contactStrengths()
	if err != nil {
		return err
	}
	data, page, err := contactsSection(a.ctx, a.tx, a.companyID, a.now, strengths)
	if err != nil {
		return err
	}
	a.out.Contacts = &struct {
		Data []crmcontracts.Company360Contact `json:"data"`
		Page crmcontracts.PageInfo            `json:"page"`
	}{Data: data, Page: page}
	return nil
}

// readStrength rides the CONTACT grant, not the company one: the
// roll-up is computed over the account's contacts, and reading an account
// does not entitle the caller to a number derived from contacts they may
// not see.
func (a *assembly) readStrength() error {
	if err := auth.Require(a.ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	strengths, err := a.contactStrengths()
	if err != nil {
		return err
	}
	a.out.Strength = accountStrengthToWire(contacts.FoldAccountStrength(strengths), a.now)
	return nil
}

func (a *assembly) readDeals() error {
	if err := auth.Require(a.ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	base, err := a.installationBaseCurrency()
	if err != nil {
		return err
	}
	section, err := dealsSection(a.ctx, a.tx, a.companyID, a.now, base)
	if err != nil {
		return err
	}
	a.out.Deals = &section
	return nil
}

// readTimeline reads the first page of the account's timeline through the
// activities module's own list, so the section and GET /activities can
// never disagree about ordering or row scope. Its gate is that list's own
// auth.Require, which refuses with the same error every other section uses
// to declare itself omitted.
func (a *assembly) readTimeline() error {
	companyUUID := a.companyID.UUID
	entityType := "company"
	limit := sectionLimit
	data, page, err := activities.ListActivitiesTx(a.ctx, a.tx, activities.ListActivitiesInput{
		EntityType:      &entityType,
		EntityID:        &companyUUID,
		Limit:           &limit,
		WithinProjectID: a.opts.ProjectID,
	})
	if err != nil {
		return err
	}
	a.out.Activities = &crmcontracts.ActivityListResponse{Data: data, Page: pageInfo(page)}
	return nil
}

// suggestionInputsOnce reads the newest message, the open pipeline and the
// scheduled-task flag ONCE. The state strip and the suggestions are two
// readings of the same three facts, and reading them twice would let the strip
// say an account is waiting while the nudge beneath it disagreed — the
// composite read exists to make that impossible.
func (a *assembly) suggestionInputsOnce() (suggestionInputs, error) {
	if !a.adviceRead {
		// The signal reading comes first so it can be handed down: the
		// contradiction rule and the health section are one query between
		// them, and the dismissal path derives its inputs from the same
		// function with its own reading.
		var facts signalFacts
		facts, a.adviceErr = a.signalFactsOnce()
		if a.adviceErr == nil {
			// The stage and the name come off the company row this
			// assembly already read, so the page adds no query for either.
			heading := companyHeading{name: a.out.Company.DisplayName}
			if lc := a.out.Company.Lifecycle; lc != nil {
				heading.lifecycle = string(*lc)
			}
			var base string
			if base, a.adviceErr = a.installationBaseCurrency(); a.adviceErr != nil {
				return a.advice, a.adviceErr
			}
			a.advice, a.adviceErr = gatherSuggestionInputs(
				a.ctx, a.tx, a.companyID, a.now, facts, heading, base, a.opts)
		}
		a.adviceRead = true
	}
	return a.advice, a.adviceErr
}

// signalFactsOnce reads the account's open signals ONCE. The health section
// counts the commitments and the contradiction rule asks whether the contract
// ended; both are the same row set, and one read of it is also what keeps the
// two from describing different instants.
func (a *assembly) signalFactsOnce() (signalFacts, error) {
	if !a.signalsRead {
		a.signals, a.signalsErr = readSignalFacts(a.ctx, a.tx, a.companyID)
		a.signalsRead = true
	}
	return a.signals, a.signalsErr
}

func (a *assembly) readNextSteps() error {
	if err := auth.Require(a.ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	data, page, err := nextStepsSection(a.ctx, a.tx, a.companyID, a.now, a.opts)
	if err != nil {
		return err
	}
	a.out.NextSteps = &struct {
		Data []crmcontracts.Company360NextStep `json:"data"`
		Page crmcontracts.PageInfo             `json:"page"`
	}{Data: data, Page: page}
	return nil
}

// readNextMeeting gates on the same grant the timeline does: a meeting IS an
// activity, and a caller who may not read the account's activities may not learn
// one is booked by another route.
func (a *assembly) readNextMeeting() error {
	if err := auth.Require(a.ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	meeting, err := nextMeetingSection(a.ctx, a.tx, a.companyID, a.now, a.opts)
	if err != nil {
		return err
	}
	a.out.NextMeeting = meeting
	return nil
}

func (a *assembly) readTags() error {
	if err := auth.Require(a.ctx, "tag", principal.ActionRead); err != nil {
		return err
	}
	tags, err := tagsSection(a.ctx, a.tx, a.companyID)
	if err != nil {
		return err
	}
	a.out.Tags = &tags
	return nil
}

// readPendingApprovals asks the approvals service, never its SQL: the
// decidability rule (authority + target visibility) is that module's, and
// a record page that re-derived it would become the workspace-wide side
// channel the inbox refuses to be. Triage is human work, so a
// passport-driven read is refused there and the section is simply absent.
func (a *assembly) readPendingApprovals() error {
	staged, triageable, err := a.pendingApprovals()
	if err != nil {
		return err
	}
	if !triageable {
		return apperrors.ErrPermissionDenied
	}
	data, page := truncate(staged)
	a.out.PendingApprovals = &struct {
		Data []crmcontracts.Approval `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: data, Page: page}
	return nil
}

func (a *assembly) readSinceLastVisit() error {
	if err := auth.Require(a.ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	delta, err := a.svc.sinceLastVisit(a.ctx, a.tx, a.companyID, a)
	if err != nil {
		return err
	}
	a.out.SinceLastVisit = &delta
	return nil
}

// pageInfo carries a store page onto the wire shape.
func pageInfo(p storekit.Page) crmcontracts.PageInfo {
	info := crmcontracts.PageInfo{HasMore: p.HasMore}
	if p.NextCursor != "" {
		info.NextCursor = &p.NextCursor
	}
	return info
}

// accountStrengthToWire adds the two account-only facts to the shared
// shape: whose relationship carries the score, and how many contacts it
// was chosen from. The shared half comes from compose.StrengthToWire, so
// a bucket rename is made once for both the account roll-up and the
// per-contact routes.
func accountStrengthToWire(account contacts.AccountStrength, now time.Time) *crmcontracts.CompanyStrength {
	base := contacts.StrengthToWire(account.RelationshipStrength, now)
	out := crmcontracts.CompanyStrength{
		Score:                   base.Score,
		Bucket:                  crmcontracts.CompanyStrengthBucket(base.Bucket),
		Factors:                 base.Factors,
		ComputedAt:              base.ComputedAt,
		LastInteraction:         base.LastInteraction,
		Inbound90d:              base.Inbound90d,
		Outbound90d:             base.Outbound90d,
		ContributingActivityIds: base.ContributingActivityIds,
		ContactCount:            account.ContactCount,
	}
	if account.ContributorContactID != nil {
		v := openapi_types.UUID(account.ContributorContactID.UUID)
		out.ContributorContactId = &v
	}
	return &out
}

// installationBaseCurrency resolves the installation's reporting currency once
// per assembly. The sections that label money each want the same answer, and
// the 360's query budget counts every read.
func (a *assembly) installationBaseCurrency() (string, error) {
	if a.baseCurrencyRead {
		return a.baseCurrency, nil
	}
	base, err := identity.BaseCurrencyOf(a.ctx, a.tx)
	if err != nil {
		return "", err
	}
	a.baseCurrency, a.baseCurrencyRead = base, true
	return base, nil
}
