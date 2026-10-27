// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Handlers is the contacts module's transport surface: the contract
// operations over contacts, companies and leads (incl. merge and
// lead promotion). Wire concerns only — decode, validate, map store
// errors to the sentinel registry; the store owns the transactional
// write shape.

import (
	"context"
	"errors"
	"net/http"
	"sync"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type Handlers struct {
	// locks reports the settings a boot source sets on each settings response;
	// nil leaves the field out.
	locks func(surface []config.Any) *[]crmcontracts.SettingLock
	// stageMatches turns suggested LinkedIn matches into approvals. See
	// WithMatchStager for why it is injected.
	stageMatches func(context.Context) error

	// stageVCardReview turns one imported card's near-match into a durable
	// create proposal. See WithVCardReviewStager for why it is injected.
	stageVCardReview func(ctx context.Context, entry VCardEntry, candidate *ids.ContactID) error

	// readCardPhoto reads a photo of business cards. Nil is an installation
	// with no model, and the photo route then answers 501.
	readCardPhoto CardPhotoReader

	store *Store
	// listMembers resolves a list read's list_id; nil while lists are off.
	listMembers storekit.ListMemberFilterResolver
	// blob serves the company logo's bytes. Nil is a role that stores no
	// objects: the logo endpoint then answers 501 rather than nil-derefing,
	// and no logo can have been resolved for it to serve anyway.
	blob blobstore.Store
	// uploadLimit is the deployment's ceiling for this module's upload route
	// (OPS-CFG-12), injected by WithUploadLimit. Zero refuses every upload,
	// which is the honest reading of "nobody has said" for a bound.
	uploadLimit int64
	// logoWritesInFlight tracks which object keys already have a trimmed-logo
	// write-back running (see streamLogo/writeBackTrimmedLogo): a pointer, so
	// every copy this builder's With* chain produces still shares the one map
	// the routes it is eventually bound to will serve concurrent requests
	// through. Allocated once here rather than lazily, so a stream that never
	// calls WithBlobstore still holds a usable, if unused, map.
	logoWritesInFlight *sync.Map
	// tightLogos is the legacy keys this process has seen need no crop, shared
	// across With* copies for the same reason as the map above.
	tightLogos *tightLogoKeys
}

// NewHandlers builds the module's HTTP surface over a workspace-bound handle.
func NewHandlers(db *database.DB) Handlers {
	return Handlers{store: NewStore(db), logoWritesInFlight: &sync.Map{}, tightLogos: newTightLogoKeys()}
}

// WithMatchStager wires the pass that turns this member's suggested LinkedIn
// matches into approvals.
//
// Injected rather than called directly because the approvals engine is a
// sibling module and this one never imports a sibling. Nil means a role that
// stages nothing — the import still runs and still matches; the suggestions
// simply wait for the hourly sweep, which is composed with the seam.
func (h Handlers) WithMatchStager(stage func(context.Context) error) Handlers {
	h.stageMatches = stage
	return h
}

// WithVCardReviewStager wires the pass that turns an imported card's
// near-match into a durable create proposal on the approvals queue.
//
// Injected rather than called directly because the approvals engine is a
// sibling module and this one never imports a sibling. Nil means a role that
// stages nothing: the import still answers needs_review per card, and the
// reader acts on the transient report alone — exactly the behaviour every
// installation had before the proposal existed.
func (h Handlers) WithVCardReviewStager(stage func(ctx context.Context, entry VCardEntry, candidate *ids.ContactID) error) Handlers {
	h.stageVCardReview = stage
	return h
}

// WithGeocodeEnqueue wires the coordinate lookup an address write queues, into
// the TRANSPORT's own store.
//
// It has to be said here as well as on the store, because the two are not the
// same object: compose builds the handlers from newContactsHandlers and keeps a
// separate s.contactsStore for the services. Wiring only the latter left every
// address written over HTTP marked stale by the trigger with no job coming —
// the enqueue existed, was correct, and was reachable from nothing a rep
// touches.
func (h Handlers) WithGeocodeEnqueue(enqueue GeocodeEnqueue) Handlers {
	h.store = h.store.WithGeocodeEnqueue(enqueue)
	return h
}

// WithVatCheckEnqueue wires the register consultation a VAT-number write
// queues, into the TRANSPORT's own store — for the reason stated above, which
// binds every enqueue this module takes rather than only the geocoder's.
func (h Handlers) WithVatCheckEnqueue(enqueue VatCheckEnqueue) Handlers {
	h.store = h.store.WithVatCheckEnqueue(enqueue)
	return h
}

// WithBlobstore wires the object store the company-logo stream reads.
func (h Handlers) WithBlobstore(blob blobstore.Store) Handlers {
	h.blob = blob
	return h
}

// WithFieldCatalog wires the workspace custom-field catalog into the
// transport's store (see Store.WithFieldCatalog); compose injects
// modules/customfields' Service here.
func (h Handlers) WithFieldCatalog(catalog fieldcatalog.Reader) Handlers {
	h.store = h.store.WithFieldCatalog(catalog)
	return h
}

// WithSettings wires the configuration the lead-settings endpoints read and
// save through.
func (h Handlers) WithSettings(cfg *config.Config) Handlers {
	h.store = h.store.WithSettings(cfg)
	return h
}

// WithSeatReadsLeads wires the identity-side grant read the escalation-seat
// check makes.
func (h Handlers) WithSeatReadsLeads(reads SeatReadsLeads) Handlers {
	h.store = h.store.WithSeatReadsLeads(reads)
	return h
}

// WithDealOpener wires the deals-side seam a qualify call opens its deal on.
func (h Handlers) WithDealOpener(opener LeadDealOpener) Handlers {
	h.store = h.store.WithDealOpener(opener)
	return h
}

// WithStopCarrier wires the consent-side seam that carries a retiring
// subject's recorded stops onto the record surviving them. Unwired, a merge
// refuses rather than dropping the stop — see stopcarry.go.
func (h Handlers) WithStopCarrier(carrier StopCarrier) Handlers {
	h.store = h.store.WithStopCarrier(carrier)
	return h
}

// WithSatelliteCarriers wires the seams that carry a retiring subject's
// consent links and introduction asks onto the survivor — see
// satellitecarry.go.
func (h Handlers) WithSatelliteCarriers(consent ConsentSatelliteCarrier, intros IntroCarrier) Handlers {
	h.store = h.store.WithSatelliteCarriers(consent, intros)
	return h
}

// WithGrantCarrier wires the seam that carries the shares on a retiring record
// onto the survivor (grantcarry.go).
func (h Handlers) WithGrantCarrier(carrier GrantCarrier) Handlers {
	h.store = h.store.WithGrantCarrier(carrier)
	return h
}

// duplicateID renders a duplicate error's existing-row pointer for the
// wire. The dedupe pre-checks leave ExistingID zero when the row is not
// visible to the caller (or a race hid it); the response then omits
// existing_id entirely — a literal zero UUID is not an id, and clients
// must never be trained to special-case one.
func duplicateID(id ids.UUID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

// duplicateLeadProblem is the 409 for a lead refused over a key another live
// lead holds, or nil when err is no such refusal.
func duplicateLeadProblem(err error) *httperr.DetailedError {
	if dupLead, ok := errors.AsType[*DuplicateLeadError](err); ok {
		return httperr.Duplicate("duplicate_email", duplicateID(dupLead.ExistingID.UUID))
	}
	if dupLeadLinkedIn, ok := errors.AsType[*DuplicateLeadLinkedInError](err); ok {
		return httperr.Duplicate("duplicate_linkedin_url", duplicateID(dupLeadLinkedIn.ExistingID.UUID))
	}
	return nil
}

// leadHoldsProblem is the 409 for a contact write over a key a live lead holds,
// or nil when err is no such refusal. It names only leads the caller may read.
func leadHoldsProblem(err error) *httperr.DetailedError {
	held, ok := errors.AsType[*LeadHoldsIdentityError](err)
	if !ok {
		return nil
	}
	if len(held.LeadIDs) == 0 {
		// Nothing the caller may read holds it: no id, no key, no convert, as
		// contact_held_by_colleague answers a hidden contact holder.
		return httperr.Duplicate("lead_holds_email", "")
	}
	leadIDs := make([]string, 0, len(held.LeadIDs))
	for _, lead := range held.LeadIDs {
		leadIDs = append(leadIDs, lead.String())
	}
	problem := httperr.Duplicate("lead_holds_email", leadIDs[0])
	problem.Details["lead_ids"] = leadIDs
	problem.Details["field"] = held.Field
	problem.Details["convertible"] = held.Convertible
	return problem
}

// contactHoldsProblem is the 409 for a lead write over a key a live contact
// holds, or nil when err is no such refusal. It names only a contact the caller
// may read, and says whether its owner was told.
func contactHoldsProblem(err error) *httperr.DetailedError {
	held, ok := errors.AsType[*ContactHoldsIdentityError](err)
	if !ok {
		return nil
	}
	problem := httperr.Duplicate(CodeContactHoldsEmail, duplicateID(held.ContactID.UUID))
	problem.Detail = held.Error()
	if problem.Details == nil {
		problem.Details = map[string]any{}
	}
	if held.Field != "" {
		problem.Details["field"] = held.Field
	}
	problem.Details["owner_notified"] = held.OwnerNotified
	return problem
}

// writeStoreErr maps this module's typed store errors onto the wire
// codes the contract names, then falls through to the sentinel registry.
func writeStoreErr(w http.ResponseWriter, r *http.Request, err error) {
	if bothProjects, ok := errors.AsType[*BothCompaniesCarryProjectsError](err); ok {
		// The names ride the body: "resolve your projects first" is only
		// actionable if the caller is told which ones.
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict,
			Code:   "both_companies_have_projects",
			Detail: bothProjects.Error(),
			Details: map[string]any{
				"source_projects": bothProjects.Source,
				"target_projects": bothProjects.Target,
			},
		})
		return
	}
	if dupEmail, ok := errors.AsType[*DuplicateEmailError](err); ok {
		httperr.Write(w, r, httperr.Duplicate("duplicate_email", duplicateID(dupEmail.ExistingID.UUID)))
		return
	}
	if held, ok := errors.AsType[*ContactHeldByColleagueError](err); ok {
		// No id and no owner: the caller learns the address is taken and
		// whether its holder was asked, nothing about the record.
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "contact_held_by_colleague", Detail: held.Error(),
			Details: map[string]any{"share_requested": held.ShareRequested},
		})
		return
	}
	if dupDomain, ok := errors.AsType[*DuplicateDomainError](err); ok {
		httperr.Write(w, r, httperr.Duplicate("duplicate_domain", duplicateID(dupDomain.ExistingID.UUID)))
		return
	}
	if dup := duplicateLeadProblem(err); dup != nil {
		httperr.Write(w, r, dup)
		return
	}
	if held := leadHoldsProblem(err); held != nil {
		httperr.Write(w, r, held)
		return
	}
	if held := contactHoldsProblem(err); held != nil {
		httperr.Write(w, r, held)
		return
	}
	if primaryConflict, ok := errors.AsType[*PrimaryConflictError](err); ok {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "multiple_primary", Detail: primaryConflict.Error(),
		})
		return
	}
	if promoted, ok := errors.AsType[*AlreadyPromotedError](err); ok {
		e := &httperr.DetailedError{
			Status: http.StatusConflict, Code: "already_promoted", Detail: promoted.Error(),
		}
		// The outcome pointer sits on the lead row the caller just proved
		// they can read, so echoing it discloses nothing new.
		if !promoted.ContactID.IsZero() {
			e.Details = map[string]any{fieldKeyPromotedContact: promoted.ContactID.String()}
		}
		httperr.Write(w, r, e)
		return
	}
	if notPromoted, ok := errors.AsType[*NotPromotedError](err); ok {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "not_promoted", Detail: notPromoted.Error(),
		})
		return
	}
	if notDisqualified, ok := errors.AsType[*NotDisqualifiedError](err); ok {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: "not_disqualified", Detail: notDisqualified.Error(),
		})
		return
	}
	if alreadyMerged, ok := errors.AsType[*AlreadyMergedError](err); ok {
		e := &httperr.DetailedError{
			Status: http.StatusConflict, Code: "already_merged", Detail: alreadyMerged.Error(),
		}
		// The redirect pointer lives on the source row the caller just proved
		// they can address, so echoing it discloses nothing new (the
		// AlreadyPromoted precedent).
		if !alreadyMerged.IntoID.IsZero() {
			e.Details = map[string]any{auditKeyMergedInto: alreadyMerged.IntoID.String()}
		}
		httperr.Write(w, r, e)
		return
	}
	httperr.Write(w, r, err)
}
