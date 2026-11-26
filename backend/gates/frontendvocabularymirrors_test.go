// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Each frontend list that mirrors a server vocabulary holds the values the
// server's list holds, and no other.
//
// A value only the server has reaches a reader as a wire enum, or as a
// capability no screen offers. A value only the browser has is a dead entry, or
// an offer the next call refuses. The server's list is the owner, and each row
// names it. A row's name is the test that held that list before the table.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/socialplatform"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// frontendVocabularyMirror is one frontend list held to the server list it
// mirrors. A marker ending in "[" opens an array or Set literal, read in order.
// Any other marker declares an object literal, whose keys are read.
type frontendVocabularyMirror struct {
	test, path, marker, owner string
	server                    func() []string
	// floor is the fewest values either side may read. Under it, a short read
	// would agree with a server that lost the same entries.
	floor   int
	ordered bool
	// missing words a value only the server has, extra one only the browser has.
	missing, extra string
	value          func(key, value string) string
}

var frontendVocabularyMirrors = []frontendVocabularyMirror{
	{
		test: "TestTheFrontendCountryListMatchesTheGoOne", owner: "contacts.AssignedCountryCodes()",
		path: "../frontend/src/format/assignedcountries.ts", marker: "ASSIGNED_COUNTRIES: readonly string[] = [",
		server:  contacts.AssignedCountryCodes,
		missing: "%s is assignable on the server and missing from the frontend list",
		extra:   "%s is in the frontend list and refused by the server, so the filter would offer a country no address can hold",
	},
	{
		test: "TestTheFrontendCurrencyListMatchesTheGoOne", owner: "values.AssignedCurrencies()",
		path: "../frontend/src/format/assignedcurrencies.ts", marker: "ASSIGNED_CURRENCIES: readonly string[] = [",
		server:  values.AssignedCurrencies,
		missing: "%s is assignable on the server and missing from the frontend list",
		extra:   "%s is in the frontend list and refused by the server, so the picker would offer a code that fails",
	},
	{
		test: "TestTheFrontendSocialPlatformsMatchTheGoList", owner: "socialplatform.Keys()",
		path: "../frontend/src/lib/socialplatforms.ts", marker: "SOCIAL_PLATFORMS = [",
		server:  socialplatform.Keys,
		missing: "the server takes social key %q and the edit form drops it, so the form cannot carry that link; change both in one commit",
		extra:   "the edit form sends social key %q and the server refuses it, so every save of a contact holding it fails; change both in one commit",
	},
	{
		test: "TestTheFrontendCustomFieldTypesMatchTheGoList", owner: "fieldcatalog.Types()",
		path: "../frontend/src/lib/customfields.logic.ts", marker: "CF_TYPES = [",
		server: fieldcatalog.Types, ordered: true,
		missing: "the server creates custom-field type %q and CF_TYPES leaves it out, so only the API can create it; change both in one commit",
		extra:   "CF_TYPES offers custom-field type %q and the server refuses it, so that create fails every time; change both in one commit",
	},
	{
		test: "TestTheFrontendMailProvidersMatchTheGoOnes", owner: "compose.MailProviders()",
		path:   "../frontend/src/format/connectorprovider.ts",
		marker: "MAIL_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([",
		server: compose.MailProviders, floor: 3,
		missing: "compose.MailProviders() serves mail ops for %q and MAIL_PROVIDERS draws no mail rows for it, a working capability with no way to reach it",
		extra:   "MAIL_PROVIDERS draws mail rows for %q and compose.MailProviders() refuses mail ops for it, so a row offers what the next call refuses",
	},
	{
		test: "TestTheFrontendRunReasonWordsMatchTheServerCodes", owner: "automation.RunReasonCodes()",
		path: "../frontend/src/lib/automationrun.ts", marker: "export const RUN_REASON_WORDS",
		server:  automation.RunReasonCodes,
		missing: "the server sends run reason %q and the browser has no words for it",
		extra:   "the browser has words for run reason %q, which the server never sends",
		value: func(code, words string) string {
			if strings.Trim(words, `"'`) != "auto.runs.reasonCode."+code {
				return fmt.Sprintf("code %q is put into the words of %s", code, words)
			}
			return ""
		},
	},
	{
		test: "TestTheFrontendExplainsEveryAutomaticTaskSource", owner: "storekit.TaskSourceFamilies()",
		path: "../frontend/src/components/taskorigin.ts", marker: "export const TASK_ORIGINS",
		server:  storekit.TaskSourceFamilies,
		missing: "%s writes tasks with no human behind them and TASK_ORIGINS has no sentence for it, so its drawer cannot say what made the task",
		extra:   "TASK_ORIGINS explains %s but storekit.TaskSourceFamilies does not name it; remove the sentence or declare the family",
	},
	{
		test: "TestEveryStageableKindHasAFrontendLabel", owner: "approvals.StageableKinds()",
		path: "../frontend/src/lib/approvalkindlabel.ts", marker: "export const KIND_LABEL",
		server:  approvals.StageableKinds,
		missing: "kind %q can be staged but has no entry in KIND_LABEL: a reader would see the wire enum",
		extra:   "KIND_LABEL carries %q, which no grant map can stage: a dead entry teaches the next author the map is not maintained",
	},
	{
		test: "TestEveryAutomatableKindHasSettingsCopy", owner: "approvals.SortedAutoApplyKinds()",
		path: "../frontend/src/features/settings/autonomy-settings.tsx", marker: "const KIND_COPY",
		server: approvals.SortedAutoApplyKinds,
		missing: "kind %q can be put on automatic but has no entry in KIND_COPY: " +
			"the rep is offered a switch labelled with the wire enum and no sentence saying what it does",
		extra: "KIND_COPY carries %q, which cannot be put on automatic: " +
			"the row it describes is never offered, and a dead entry teaches the next author the map is not maintained",
	},
	{
		test: "TestTheFrontendDealRolesMatchTheGoVocabulary", owner: "dealrole.Shown",
		path: "../frontend/src/lib/recordlabels.ts", marker: "const DEAL_ROLE_LABELS",
		server:  func() []string { return dealrole.Shown },
		missing: "dealrole.Shown names role %q and DEAL_ROLE_LABELS leaves it out, so the meeting brief and the contact page disagree on it",
		extra:   "DEAL_ROLE_LABELS names role %q and dealrole.Shown does not, so the meeting brief and the contact page disagree on it",
	},
	{
		test: "TestTheFrontendDealRolesMatchTheGoVocabulary_legacy", owner: "dealrole.LegacyDecisionMaker",
		path: "../frontend/src/lib/recordlabels.ts", marker: "const LEGACY_DEAL_ROLE_LABELS",
		server:  func() []string { return []string{dealrole.LegacyDecisionMaker} },
		missing: "dealrole keeps legacy role %q readable and LEGACY_DEAL_ROLE_LABELS leaves it out, so the meeting brief and the contact page disagree on it",
		extra:   "LEGACY_DEAL_ROLE_LABELS names role %q and dealrole has no such legacy role, so the meeting brief and the contact page disagree on it",
	},
}

func TestEveryFrontendVocabularyMirrorMatchesTheServer(t *testing.T) {
	t.Parallel()
	for _, mirror := range frontendVocabularyMirrors {
		t.Run(mirror.test, func(t *testing.T) {
			t.Parallel()
			mirror.check(t)
		})
	}
}

// check fails each value one side has and the other lacks, in both directions.
func (m frontendVocabularyMirror) check(t *testing.T) {
	t.Helper()
	inTS, inGo := m.browserValues(t), m.server()
	floor := max(m.floor, 1)
	if len(inTS) < floor {
		t.Fatalf("%s: read %d values after %q (%v), under the floor of %d, so the read has gone short "+
			"and this gate would pass on a half-empty file", m.path, len(inTS), m.marker, inTS, floor)
	}
	if len(inGo) < floor {
		t.Fatalf("%s = %v, under the floor of %d, so the comparison would hold by agreeing with the loss",
			m.owner, inGo, floor)
	}
	for _, value := range inGo {
		if !slices.Contains(inTS, value) {
			t.Errorf("%s: %s", m.path, fmt.Sprintf(m.missing, value))
		}
	}
	for _, value := range inTS {
		if !slices.Contains(inGo, value) {
			t.Errorf("%s: %s", m.path, fmt.Sprintf(m.extra, value))
		}
	}
	if m.ordered && !t.Failed() && !slices.Equal(inTS, inGo) {
		t.Errorf("%s lists %v and %s lists %v; the screen shows them in this order, so change both in one commit",
			m.path, inTS, m.owner, inGo)
	}
}

// browserValues reads the frontend side: an array in order, or a map's keys.
func (m frontendVocabularyMirror) browserValues(t *testing.T) []string {
	t.Helper()
	if strings.HasSuffix(m.marker, "[") {
		return tsArrayStrings(t, m.path, m.marker)
	}
	entries := tsObjectEntries(t, m.path, m.marker)
	keys := sortedKeys(tsKeySet(t, entries))
	if m.value != nil {
		for _, key := range keys {
			if problem := m.value(key, entries[key]); problem != "" {
				t.Errorf("%s: %s", m.path, problem)
			}
		}
	}
	return keys
}

// tsArrayStrings reads the string entries of the array literal marker opens, in
// order. An entry that is no string literal, or a string listed twice, fails:
// either would let the read differ from the list the screen holds.
func tsArrayStrings(t *testing.T, path, marker string) []string {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := tsWithoutComments(string(source))
	at := indexAfter(text, marker)
	if at < 0 {
		t.Fatalf("%s no longer declares %q as an array literal, so this gate reads a shape that is gone", path, marker)
	}
	body, ok := tsBalanced(text[at-1:])
	if !ok {
		t.Fatalf("%s: the literal after %q is unterminated", path, marker)
	}
	var values []string
	for _, entry := range tsTopLevel(body) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		quote := entry[0]
		if len(entry) < 2 || (quote != '"' && quote != '\'') || entry[len(entry)-1] != quote ||
			strings.IndexByte(entry[1:len(entry)-1], quote) >= 0 {
			t.Errorf("%s: the list after %q holds %s, which is no string literal this gate can read; write the value out", path, marker, entry)
			continue
		}
		value := entry[1 : len(entry)-1]
		if slices.Contains(values, value) {
			t.Errorf("%s: the list after %q holds %q twice", path, marker, value)
		}
		values = append(values, value)
	}
	return values
}
