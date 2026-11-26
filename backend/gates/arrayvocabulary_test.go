// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A text array either closes its set in the database or says why its set is open.
//
// PostgreSQL has no SET type, so an array is how a row holds several words from
// a fixed list, and `col <@ ARRAY[…]` is what closes that list. A column whose
// list lives only in a Go writer admits any word from any other writer. The
// census reads every text array column from the committed head catalog; a
// column with no containment or shape CHECK carries a waiver saying why.
//
// A containment CHECK that copies a Go list is held equal to that list here,
// so the two cannot drift.

import (
	"bufio"
	"bytes"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/workarea"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// textArrayColumnFloor is below the count of text array columns the schema
// carries, so a catalog read that stopped seeing them fails.
const textArrayColumnFloor = 40

// catalogTextArrayColumn reads one text or varchar array column line.
var catalogTextArrayColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) (?:text|character varying(?:\(\d+\))?)\[\]`)

// containedIn reads each `col <@ ARRAY[…]` inside a CHECK body.
var containedIn = regexp.MustCompile(`([a-z_][a-z0-9_]*) <@ ARRAY\[([^\]]*)\]`)

// shapedBy reads each `array_to_string(col, …) ~ …` inside a CHECK body: a
// pattern every element must match, such as an ISO country code.
var shapedBy = regexp.MustCompile(`array_to_string\(([a-z_][a-z0-9_]*),`)

// addressesAreOpen is the reason every array of mail addresses stays open.
const addressesAreOpen = "mail addresses a message or a request named; any address is a valid entry, so no list can close them"

// openTextArrays are the text arrays whose set the database does not close.
var openTextArrays = gatekit.Waive(map[string]string{
	"raw_capture.channel_subjects":                        "channel identities as `provider:account`, written by every network that stores a raw original; the providers own the set",
	"activity.redacted_fields":                            "names of activity columns, written by the privacy erasure paths; the set moves with the schema",
	"comms_outbound.redacted_fields":                      "names of comms_outbound columns, written by the privacy erasure paths; the set moves with the schema",
	"capture_connection.provider_scopes":                  "the OAuth scopes the provider granted, stored as it returned them; the provider owns the set",
	"capture_import.delivered_by":                         "mailbox marks, a provider or connection id with its generation",
	"capture_import.provider_message_ids":                 "message ids the provider assigned",
	"capture_import.verdict_reasons":                      "audience reasons the capture sink writes from its constants in capture/sinkaudience.go, and nothing else writes it",
	"capture_party_source.containers":                     "provider folder and label ids",
	"capture_party_source.policy_addresses":               addressesAreOpen,
	"capture_party_source.recipients":                     addressesAreOpen,
	"capture_thread_verdict.seen_addresses":               addressesAreOpen,
	"communication_decision.ruleset_codes":                "jurisdiction pack codes, one per pack an extension registers at runtime",
	"communication_suppression.recorded_for_addresses":    addressesAreOpen,
	"consent_qualifying_event.recorded_for_addresses":     addressesAreOpen,
	"contact.anonymized_address_hashes":                   "hashes of erased addresses",
	"contact_acquisition_evidence.recorded_for_addresses": addressesAreOpen,
	"contact_consent.recorded_for_addresses":              addressesAreOpen,
	"deal_correction.fields":                              "names of deal columns a correction moved; the set moves with the schema",
	"mail_draft.bcc_addresses":                            addressesAreOpen,
	"mail_draft.cc_addresses":                             addressesAreOpen,
	"mail_draft.to_addresses":                             addressesAreOpen,
	"oauth_authorization_code.scopes":                     "passport scopes plus the offline_access marker, which parseOAuthScopes admits and the token exchange removes",
	"oauth_client.redirect_uris":                          "URIs a client registers",
	"onboarding_wizard_state.selected_fact_keys":          "keys of facts read from the company's own site",
	"partner.served_segments":                             "free text a partner describes itself with",
	"privacy_notice_case.allowed_routes":                  "notice routes from consent/noticerule.go, which the notice case writer alone sets",
	"privacy_request_proof.erased_uncounted":              "object kinds an erasure destroyed outside a row count, including the legacy per_table_rows proofs keep",
	"provider_connection.categories":                      "each provider declares its own categories, checked by the provider's descriptor",
	"provider_run.requested_categories":                   "each provider declares its own categories, checked by the provider's descriptor",
	"voice_profile_version.review_reasons":                "translated sentences, kept in the language they were written in",
	"web_form.allowed_origins":                            "browser origins a form admits",
	"webhook_subscription.event_types":                    "event names from the generated event catalog, which the webhook store checks; a CHECK would be a third copy of a list that grows with every event",
})

// arrayVocabularySources names the Go list each containment CHECK copies.
var arrayVocabularySources = map[string]func(t *testing.T) []string{
	"passport.scopes":                     principalScopes,
	"oauth_grant.scopes":                  principalScopes,
	"passport.work_areas":                 workAreaKeys,
	"oauth_grant.work_areas":              workAreaKeys,
	"oauth_authorization_code.work_areas": workAreaKeys,
	"custom_field.activity_kinds":         typedSet("internal/contracts.ActivityKind"),
	"deal.manager_request_reasons":        func(*testing.T) []string { return asSet(deals.ManagerRequestReasons) },
}

func principalScopes(t *testing.T) []string {
	return typedSet("internal/shared/kernel/principal.Scope")(t)
}

func workAreaKeys(*testing.T) []string {
	var keys []string
	for _, area := range workarea.Embedded().Areas {
		keys = append(keys, area.Key)
	}
	return asSet(keys)
}

// typedSet answers the values of one Go string type, read by the vocabulary census.
func typedSet(key string) func(t *testing.T) []string {
	return func(t *testing.T) []string {
		t.Helper()
		typed, err := goTypedSets()
		if err != nil {
			t.Fatal(err)
		}
		values, ok := typed[key]
		if !ok {
			t.Fatalf("no Go constants of type %s: the type moved, so move this source with it", key)
		}
		return values
	}
}

// arrayCatalog is what the catalog says about text array columns: every one,
// the containment list each CHECK gives it, and which ones a shape holds.
type arrayCatalog struct {
	columns   []string
	contained map[string][]string
	shaped    map[string]bool
}

func readArrayCatalog(t *testing.T) arrayCatalog {
	t.Helper()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	out := arrayCatalog{contained: map[string][]string{}, shaped: map[string]bool{}}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := catalogTextArrayColumn.FindStringSubmatch(line); m != nil {
			out.columns = append(out.columns, m[1]+"."+m[2])
			continue
		}
		m := catalogCheckLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, c := range containedIn.FindAllStringSubmatch(m[2], -1) {
			var values []string
			for _, lit := range catalogTextLiteral.FindAllStringSubmatch(c[2], -1) {
				values = append(values, strings.ReplaceAll(lit[1], "''", "'"))
			}
			out.contained[m[1]+"."+c[1]] = asSet(values)
		}
		for _, s := range shapedBy.FindAllStringSubmatch(m[2], -1) {
			out.shaped[m[1]+"."+s[1]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out.columns) < textArrayColumnFloor {
		t.Fatalf("read %d text array columns from %s, fewer than the %d the schema carries: the catalog format moved",
			len(out.columns), vocabularyCatalog, textArrayColumnFloor)
	}
	return out
}

func TestATextArrayClosesItsSetOrSaysWhyItIsOpen(t *testing.T) {
	t.Parallel()
	catalog := readArrayCatalog(t)
	for _, column := range catalog.columns {
		if _, closed := catalog.contained[column]; closed || catalog.shaped[column] {
			continue
		}
		if openTextArrays.Waived(t, column) {
			continue
		}
		t.Errorf("%s is a text array with no containment CHECK. Close its set with `%s <@ ARRAY[…]` and name its Go source in arrayVocabularySources, or waive it here with the reason its set is open",
			column, column[strings.IndexByte(column, '.')+1:])
	}
	openTextArrays.AssertAllMatched(t)
}

func TestAnArrayVocabularyMatchesTheGoListItCopies(t *testing.T) {
	t.Parallel()
	catalog := readArrayCatalog(t)
	for column, source := range arrayVocabularySources {
		stored, ok := catalog.contained[column]
		if !ok {
			t.Errorf("%s names a Go source here, but the catalog holds no containment CHECK on it", column)
			continue
		}
		if want := source(t); !slices.Equal(stored, want) {
			t.Errorf("%s admits %v, but its Go source holds %v. Change both in one migration", column, stored, want)
		}
	}
}
