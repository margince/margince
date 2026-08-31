// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The "Provided by Surfe" snapshot (ADR-0101, PO-EXT-9): what a licensed data
// provider was paid to tell us about this person, kept BESIDE the canonical
// record and never folded into it.
//
// The fold is a UNION over every retained completed run, not newest-per-key.
// After a merge the survivor holds both sides' purchases, and both were paid
// for (PI-AC-11) — newest-per-key would silently discard the losing side's
// answer, which is data the customer bought. Where two runs assert the same
// category they stand as peer assertions, each with its own provider and
// retrieval time, and contributing_runs names every run the section drew on.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// providerProfileSection assembles one snapshot per CONNECTED provider.
//
// One section per connection rather than one for "the provider": the reader
// decides who to spend money with, and a page that named nobody left them
// unable to tell who had already been paid for the number on screen. A
// provider nobody has run yet is present with never_run, because an absent
// section is a verb the reader cannot reach.
//
// The gate is the PERSON read: a claim is a fact about this person, bought
// about them, so seeing it requires seeing them. No separate grant exists and
// none should — a rep who may open the record may see what the installation
// paid to learn about it.
func (s *Service) providerProfileSection(ctx context.Context, tx pgx.Tx, personID ids.PersonID, out *crmcontracts.Person360) error {
	if err := requireRead(ctx, "person"); err != nil {
		return err
	}
	statuses, err := s.connectionStatuses(ctx, tx)
	if err != nil {
		return err
	}
	runs, err := s.providerRuns(ctx, tx, personID)
	if err != nil {
		return err
	}
	claims, err := s.storedClaims(ctx, tx, personID)
	if err != nil {
		return err
	}
	// A provider this person has runs or purchases under, but which nobody is
	// connected to any more, still gets a section: disconnecting stops new
	// egress, it does not delete what was bought, and a page that dropped the
	// section would hide a purchase the customer paid for.
	//
	// Set even when empty — a pointer to no sections says "nobody is
	// connected", which is a different fact from the nil the grant check
	// leaves behind, and `sections_omitted` is what names the second.
	profiles, err := s.profilesFor(namesToShow(statuses, runs, claims), statuses, runs, claims)
	if err != nil {
		return err
	}
	out.ProviderProfiles = &profiles
	return nil
}

// connectionStatuses is each provider's connection status, read from the same
// rows the settings card reads: a page that said "never run" while the card
// said "not connected" would have the reader looking for a button that is not
// there. No registry is consulted — a connection row exists only where an
// adapter registered one.
//
// The STATUS, not merely whether one is connected. A connection can be present
// and impaired — the key was refused, the credits ran out, the vendor asked us
// to slow down — and each of those is something an operator can act on. Read as
// a boolean they all collapsed into "not connected", which the page then
// reported as `stale`: "bought earlier, nobody is connected any more". That
// sends somebody looking at the contact for a problem that is in the settings.
func (s *Service) connectionStatuses(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	rows, err := tx.Query(ctx, `SELECT provider, status FROM provider_connection`)
	if err != nil {
		return nil, fmt.Errorf("person360: reading the provider connection state: %w", err)
	}
	defer rows.Close()
	statuses := map[string]string{}
	for rows.Next() {
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			return nil, fmt.Errorf("person360: scanning a connected provider: %w", err)
		}
		statuses[name] = status
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("person360: reading the provider connection state: %w", err)
	}
	return statuses, nil
}

// impairedStates maps a connection that EXISTS but cannot buy onto the sentence
// the page shows. Each names the installation's own condition, because that is
// where the fix is — none of them is a fact about the contact.
var impairedStates = map[string]crmcontracts.PersonProviderProfileState{
	"invalid_credentials":  crmcontracts.PersonProviderProfileStateInvalidCredentials,
	"insufficient_credits": crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"rate_limited":         crmcontracts.PersonProviderProfileStateRateLimited,
	"provider_error":       crmcontracts.PersonProviderProfileStateProviderError,
}

// namesToShow collects the providers the page owes the reader a section for:
// the connected ones, plus any that already hold a run or a purchase here.
// Sorted, so the sections keep their order between reads rather than
// reshuffling under somebody mid-click.
//
// Held by: TestAPurchaseSurvivesItsProviderBeingDisconnected and
// TestSectionsKeepAStableOrderBetweenReads
// (internal/compose/person360/providerpartition_test.go) — the first pins that
// a provider with history but no connection is still named, the second the
// order.
func namesToShow(statuses map[string]string, runs []providerRunRow, claims []storedClaim) []string {
	seen := map[string]bool{}
	for name := range statuses {
		seen[name] = true
	}
	for _, r := range runs {
		seen[r.providerName] = true
	}
	for _, c := range claims {
		seen[c.provider] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// profilesFor builds one snapshot per named provider, each seeing only its own
// runs and its own claims. The partition is the whole point: a value bought
// from one vendor must never appear under another's name.
func (s *Service) profilesFor(names []string, statuses map[string]string, runs []providerRunRow, claims []storedClaim) ([]crmcontracts.PersonProviderProfile, error) {
	profiles := make([]crmcontracts.PersonProviderProfile, 0, len(names))
	for _, name := range names {
		profile, err := s.profileFor(name, statuses[name], runsOf(name, runs), claimsOf(name, claims))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

// profileFor is one provider's section: what it was asked, what it answered,
// and what it sold us.
func (s *Service) profileFor(name string, status string, runs []providerRunRow, claims []storedClaim) (crmcontracts.PersonProviderProfile, error) {
	profile := crmcontracts.PersonProviderProfile{
		Provider:               crmcontracts.Provider(name),
		State:                  resolveProviderState(runs, status),
		CategoriesNotRequested: []string{},
		Emails:                 []crmcontracts.PersonProviderEmail{},
		MobilePhones:           []crmcontracts.PersonProviderPhone{},
		JobHistory:             []crmcontracts.PersonProviderJobHistory{},
		Departments:            []string{},
		Seniorities:            []string{},
	}
	if len(runs) > 0 {
		latest := runs[0]
		profile.RetrievedAt = latest.completedAt
		if latest.safeCode != "" {
			profile.SafeStatusCode = providerPtr(latest.safeCode)
		}
		profile.LatestRun = providerPtr(toWireRun(latest))
		profile.ContributingRuns = providerPtr(contributingRuns(runs))
		if s.providers != nil {
			if desc, err := s.providers.Descriptor(name); err == nil {
				profile.CategoriesNotRequested = categoriesNotRequested(desc, runs, claims)
				if answerable(latest) {
					delivered := deliveredKeys(latest.id, claims)
					profile.CategoriesAsked = providerPtr(asked(desc, latest.requested, delivered))
					profile.CategoriesWithoutAnswer = providerPtr(categoriesWithoutAnswer(
						desc, latest.requested, delivered))
				}
			}
		}
	}
	if err := foldClaims(claims, &profile); err != nil {
		return crmcontracts.PersonProviderProfile{}, err
	}
	return profile, nil
}

// runsOf and claimsOf narrow the person's history to one provider, preserving
// the order the reads established: runs newest-first, claims oldest-first.
func runsOf(name string, runs []providerRunRow) []providerRunRow {
	var out []providerRunRow
	for _, r := range runs {
		if r.providerName == name {
			out = append(out, r)
		}
	}
	return out
}

func claimsOf(name string, claims []storedClaim) []storedClaim {
	var out []storedClaim
	for _, c := range claims {
		if c.provider == name {
			out = append(out, c)
		}
	}
	return out
}

// providerRunRow is one run as this section reads it: the lifecycle facts and
// what the run was allowed to ask for. Not the frozen snapshot, the
// correlation id or the reservations — none of them is a fact about the
// PERSON, and the run endpoint serves them to anyone who needs them.
type providerRunRow struct {
	id                ids.UUID
	providerName      string
	trigger           string
	connectionVersion int64
	state             string
	claimsUnwritten   bool
	// Whether this run's answers have been offered to the record. The page
	// waits on it: the apply commits AFTER the run completes, so a client
	// that stopped at the terminal state would refresh one step before the
	// values it is waiting for exist.
	applied     bool
	safeCode    string
	requested   []string
	skipReason  string
	createdAt   time.Time
	completedAt *time.Time
}

// providerRuns reads this person's run history, newest first. Scrubbed runs
// are gone from it by construction: an erasure nulls person_id, so a run that
// no longer names anybody cannot be read back onto their page.
func (s *Service) providerRuns(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]providerRunRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, provider, trigger, connection_version, state, claims_unwritten,
		       applied_at IS NOT NULL,
		       coalesce(last_safe_status_code, ''), coalesce(skip_reason, ''),
		       requested_categories, created_at, completed_at
		  FROM provider_run
		 WHERE person_id = $1 AND subject_kind = 'person'
		 ORDER BY created_at DESC`, personID)
	if err != nil {
		return nil, fmt.Errorf("person360: reading the provider runs: %w", err)
	}
	defer rows.Close()
	var out []providerRunRow
	for rows.Next() {
		var r providerRunRow
		if err := rows.Scan(&r.id, &r.providerName, &r.trigger, &r.connectionVersion, &r.state, &r.claimsUnwritten,
			&r.applied, &r.safeCode, &r.skipReason, &r.requested, &r.createdAt, &r.completedAt); err != nil {
			return nil, fmt.Errorf("person360: scanning a provider run: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("person360: reading the provider runs: %w", err)
	}
	return out, nil
}

// toWireRun renders one run for the page: which provider, what was asked for,
// how it ended.
//
// The reservations and the frozen configuration snapshot are deliberately
// left empty. What a run COST is the settings card's subject, and a credit
// figure beside somebody's mobile number invites reading one as the price of
// the other; the run endpoint serves both to a caller who actually needs
// them.
func toWireRun(r providerRunRow) crmcontracts.ProviderRun {
	return crmcontracts.ProviderRun{
		Id:                  openapi_types.UUID(r.id),
		SubjectKind:         crmcontracts.ProviderRunSubjectKindPerson,
		Provider:            crmcontracts.Provider(r.providerName),
		Trigger:             crmcontracts.ProviderRunTrigger(r.trigger),
		State:               crmcontracts.ProviderRunState(r.state),
		ClaimsUnwritten:     r.claimsUnwritten,
		Applied:             &r.applied,
		RequestedCategories: r.requested,
		ConnectionVersion:   r.connectionVersion,
		CreatedAt:           r.createdAt,
		UpdatedAt:           r.createdAt,
		CompletedAt:         r.completedAt,
	}
}

// contributingRuns names every run whose claims this snapshot drew on: the
// completed ones that delivered values. Normally the single latest run; after
// a merge it spans BOTH sides, because both were paid for and the section
// shows both (PI-AC-11).
func contributingRuns(runs []providerRunRow) []crmcontracts.ProviderRun {
	out := []crmcontracts.ProviderRun{}
	for _, r := range runs {
		if r.state == string(provider.RunCompleted) && !r.claimsUnwritten {
			out = append(out, toWireRun(r))
		}
	}
	return out
}

// categoriesNotRequested is what nobody asked for — the difference between the
// provider's full vocabulary and what has been requested for this contact. It
// is the page's answer to a blank field: "we never asked" is a different fact
// from "we asked and they had nothing", and only this list tells them apart.
//
// Two things take a category off this list, and they answer the question from
// opposite ends. The LATEST run's own requests count whatever they returned,
// because its receipt is rendered beside this list and already says what came
// back empty. And any category whose value is ON THE PAGE counts, whatever the
// run history says.
//
// The second is what stops the section contradicting itself. Reading runs alone
// printed "never asked for: mobile number" directly above a mobile number,
// because claims outlive the runs that fetched them: a merge relinks the losing
// side's purchases to the survivor, and a seeded or imported installation holds
// claims with no run at all.
//
// Counting every earlier RUN instead would open the opposite gap:
// categories_without_answer is the latest run's receipt by contract, so a
// category an older run asked about and got nothing for would leave this list
// without entering that one, and vanish from the page entirely.
func categoriesNotRequested(desc provider.Descriptor, runs []providerRunRow, claims []storedClaim) []string {
	// Claim KEYS, which are not category names — `professional_email` is asked
	// for and `professional_emails` comes back. desc.Answers owns that mapping
	// and answeredBy applies it; comparing the two vocabularies directly
	// matches nothing and silently reports every category as unanswered.
	delivered := map[string]bool{}
	for _, c := range claims {
		delivered[c.key] = true
	}
	asked := map[string]bool{}
	for _, r := range runs {
		for _, c := range r.requested {
			// The latest run's own requests count whatever it returned: its
			// receipt is rendered beside this list, so a category it asked
			// about and got nothing for is already accounted for there.
			if r.id == runs[0].id {
				asked[c] = true
			}
		}
	}
	out := []string{}
	for _, c := range desc.Categories {
		// A category whose value is ON THE PAGE is never one nobody asked for,
		// whatever the run history says. Claims outlive the runs that fetched
		// them — a merge relinks the losing side's purchases to the survivor,
		// and a seeded or imported installation holds claims with no run at all
		// — so deciding this from runs alone printed "never asked for: mobile
		// number" directly above the mobile number.
		if asked[string(c)] || answeredBy(desc.Answers[c], delivered) {
			continue
		}
		out = append(out, string(c))
	}
	sort.Strings(out)
	return out
}

// resolveProviderState answers the ONE sentence the page shows about this
// person's enrichment. The three "nothing here" states are three different
// facts and must never collapse into one: nobody connected a provider, this
// person is not eligible for one, and nobody has asked yet are different
// answers to "why is this empty", and only the reader can act on the right
// one.
func resolveProviderState(runs []providerRunRow, status string) crmcontracts.PersonProviderProfileState {
	// A connection that exists but cannot buy says WHY, before anything about
	// this person's runs. The condition is the installation's — a refused key,
	// an exhausted wallet — and it outranks the run history because it is what
	// the next lookup will hit, and the only thing anybody can act on.
	if impaired, ok := impairedStates[status]; ok {
		return impaired
	}
	if status != "connected" {
		// Disconnected, and the data this installation already PAID for is
		// still on the page below — disconnecting stops new egress, it does
		// not delete what was bought. So the honest word is stale, not
		// not_connected: telling the reader the platform knows nothing while
		// showing them a purchased mobile number is the one thing this state
		// exists to prevent. not_connected is for a page with nothing on it.
		if len(runs) > 0 {
			return crmcontracts.PersonProviderProfileStateStale
		}
		return crmcontracts.PersonProviderProfileStateNotConnected
	}
	if len(runs) == 0 {
		return crmcontracts.PersonProviderProfileStateNeverRun
	}
	latest := runs[0]
	if latest.state == string(provider.RunCompleted) && latest.claimsUnwritten {
		// Paid, and the values never reached the record. Its own state
		// because it is neither a success nor a failure: the customer was
		// charged and has nothing to show for it, which is a thing somebody
		// needs to see rather than a completed run with empty fields.
		return crmcontracts.PersonProviderProfileStateCompletedClaimsUnwritten
	}
	if latest.state == string(provider.RunSkipped) {
		// A skip is a decision, and its REASON is what the reader needs. An
		// exhausted budget is a fact about the installation's wallet; telling
		// somebody "this person is not eligible" instead would send them
		// looking at the contact for a problem that is not there.
		return skipStates[latest.skipReason]
	}
	if mapped, ok := providerRunStates[latest.state]; ok {
		return mapped
	}
	return crmcontracts.PersonProviderProfileStateProviderError
}

// skipStates says WHY nothing was bought, in the page's vocabulary. The zero
// value is not_eligible, which is the honest default for the two reasons that
// really are about the subject; everything else names the installation's own
// condition instead.
var skipStates = map[string]crmcontracts.PersonProviderProfileState{
	"":                            crmcontracts.PersonProviderProfileStateNotEligible,
	"not_eligible":                crmcontracts.PersonProviderProfileStateNotEligible,
	"duplicate_subject_candidate": crmcontracts.PersonProviderProfileStateNotEligible,
	// A consent decision, not an eligibility one. It reads as not_eligible
	// today because the contract has no suppressed state; the difference is
	// worth a spec change rather than a wrong word invented here.
	"suppressed":       crmcontracts.PersonProviderProfileStateNotEligible,
	"budget_exhausted": crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"low_balance":      crmcontracts.PersonProviderProfileStateInsufficientCredits,
	"rate_limited":     crmcontracts.PersonProviderProfileStateRateLimited,
	// Nothing was bought because we already hold an answer inside the refresh
	// window — which is a completed enrichment, not a refusal.
	"already_fresh": crmcontracts.PersonProviderProfileStateCompleted,
}

// providerRunStates maps the run machine onto the page's vocabulary. The two
// are deliberately separate: a run state is what the platform is doing, and
// this is what the reader is told.
var providerRunStates = map[string]crmcontracts.PersonProviderProfileState{
	string(provider.RunQueued):            crmcontracts.PersonProviderProfileStateQueued,
	string(provider.RunSubmitting):        crmcontracts.PersonProviderProfileStateInProgress,
	string(provider.RunInProgress):        crmcontracts.PersonProviderProfileStateInProgress,
	string(provider.RunCompleted):         crmcontracts.PersonProviderProfileStateCompleted,
	string(provider.RunNoMatch):           crmcontracts.PersonProviderProfileStateNoMatch,
	string(provider.RunSubmissionUnknown): crmcontracts.PersonProviderProfileStateSubmissionUnknown,
	string(provider.RunCancelled):         crmcontracts.PersonProviderProfileStateNeverRun,
}
