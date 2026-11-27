// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Each limit or marker a frontend file copies from the server equals its owner.
// Only those copies are listed; a copy gone, doubled or changed fails.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// frontendLimit is one value a frontend file spells and the pattern that reads it.
// owner answers what it must equal; drift is what a reader meets when they part.
type frontendLimit struct {
	file  string
	name  string
	read  *regexp.Regexp
	owner func(t *testing.T, contract map[string]any) []string
	drift string
}

const screenRefusesApart = "the screen and the server refuse different values"

// tsNumeral is a number literal; its `_` digit separators are not part of the value.
var tsNumeral = regexp.MustCompile(`^[0-9][0-9_]*$`)

func TestTheFrontendsCopiesOfServerLimitsMatchTheirOwners(t *testing.T) {
	t.Parallel()
	contract := loadContractDocument(t)
	limits := []frontendLimit{
		{
			file: "../frontend/src/components/create.fields.ts", name: "EARLIEST_DAY",
			read:  regexp.MustCompile(`\bEARLIEST_DAY\s*=\s*"([^"]*)"\s*;`),
			owner: fixed(datasource.AcceptedInstantMin),
			drift: screenRefusesApart,
		},
		{
			file: "../frontend/src/components/create.fields.ts", name: "LATEST_DAY",
			read:  regexp.MustCompile(`\bLATEST_DAY\s*=\s*"([^"]*)"\s*;`),
			owner: fixed(datasource.AcceptedInstantMax),
			drift: screenRefusesApart,
		},
		{
			file: "../frontend/src/lib/installationcompany.ts", name: "COMPANY_NAME_MAX",
			read: regexp.MustCompile(`\bCOMPANY_NAME_MAX\s*=\s*(\d+)\s*;`),
			// The anchor company's write refuses a name past this.
			owner: fixed(strconv.Itoa(values.TextLabelCharacters)),
			drift: screenRefusesApart,
		},
		{
			file: "../frontend/src/features/settings/rates.tsx", name: "RATE_DIGITS",
			read:  regexp.MustCompile(`\bRATE_DIGITS\s*=\s*(\d+)\s*;`),
			owner: fxRateDigits,
			drift: screenRefusesApart,
		},
		{
			file: "../frontend/src/design-system/rowtags.tsx", name: "WIRE_ROW_TAG_CAP",
			read:  regexp.MustCompile(`const WIRE_ROW_TAG_CAP\s*=\s*(\d+)\s*;`),
			owner: fixed(strconv.Itoa(storekit.RowTagCap)),
			drift: "a row page carries storekit.RowTagCap tags, so the strip counts the rest off an array that stopped early",
		},
		{
			file: "../frontend/src/lib/voice-refusal.ts", name: "VOICE_SOURCE_MAX_BYTES",
			read:  regexp.MustCompile(`(?m)^export const VOICE_SOURCE_MAX_BYTES = ([0-9_]+);`),
			owner: fixed(strconv.Itoa(httperr.MaxBodyBytes)),
			drift: "the server reads a JSON write up to httperr.MaxBodyBytes, so change both together",
		},
		{
			file: "../frontend/src/features/privacy/retention.logic.ts", name: "RETAIN_DAYS.max",
			read:  regexp.MustCompile(`(?m)^export const RETAIN_DAYS = \{\s*min:\s*\d+,\s*max:\s*(\d+)\s*\} as const;`),
			owner: fixed(strconv.Itoa(privacy.RetainDaysCeiling)),
			drift: "the editor refuses windows privacy.RetainDaysCeiling stores, or sends one only a 422 answers",
		},
		{
			file: "../frontend/src/lib/segmentpredicate.ts", name: "MAX_DAYS_AGO",
			read:  regexp.MustCompile(`(?m)^export const MAX_DAYS_AGO = ([0-9_]+);`),
			owner: fixed(strconv.Itoa(storekit.MaxRelativeDays)),
			drift: "the filter builder and storekit.MaxRelativeDays bound days ago apart, so move both together",
		},
		{
			file: "../frontend/src/format/calendarday.ts", name: "DUE_YEARS_BACK",
			read:  regexp.MustCompile(`export const DUE_YEARS_BACK = (\d+);`),
			owner: fixed(strconv.Itoa(values.DueYearsBack)),
			drift: "the due-date box offers days outside the window values.DueYearsBack lets the server accept",
		},
		{
			file: "../frontend/src/format/calendarday.ts", name: "DUE_YEARS_AHEAD",
			read:  regexp.MustCompile(`export const DUE_YEARS_AHEAD = (\d+);`),
			owner: fixed(strconv.Itoa(values.DueYearsAhead)),
			drift: "the due-date box offers days outside the window values.DueYearsAhead lets the server accept",
		},
		{
			file: "../frontend/src/components/bulkchangeapi.ts", name: "BULK_CONFIRM_ABOVE",
			read:  regexp.MustCompile(`(?m)^export const BULK_CONFIRM_ABOVE\s*=\s*(\d+);`),
			owner: bulkConfirmAbove,
			drift: "a change the server confirms as large runs with no Large change warning, so change both together",
		},
		{
			file: "../frontend/src/features/booking/meeting-link-drawer.tsx", name: "TITLE_MAX",
			read:  regexp.MustCompile(`const TITLE_MAX = (\d+);`),
			owner: maxLength("components", "schemas", "SchedulingProfile", "properties", "title"),
			drift: "the Meetings screen stops a title at a length SchedulingProfile.title does not declare",
		},
		{
			file: "../frontend/src/features/booking/meeting-link-drawer.tsx", name: "LOCATION_MAX",
			read:  regexp.MustCompile(`const LOCATION_MAX = (\d+);`),
			owner: maxLength("components", "schemas", "SchedulingProfile", "properties", "location"),
			drift: "the Meetings screen stops a location at a length SchedulingProfile.location does not declare",
		},
		{
			file: "../frontend/src/features/leads/leadsignals.tsx", name: "MANUAL_SIGNAL_NO_EVIDENCE",
			read:  regexp.MustCompile(`export const MANUAL_SIGNAL_NO_EVIDENCE\s*=\s*"([^"\n]*)";`),
			owner: fixed(contacts.ManualSignalNoEvidence),
			drift: "the browser misses contacts.ManualSignalNoEvidence, so stored signals show untranslated",
		},
		{
			file: "../frontend/src/features/automations/remoteimagebox.ts", name: "REMOTE_IMAGE_BOX",
			read:  regexp.MustCompile(`export const REMOTE_IMAGE_BOX\s*=\s*["']([^"'\n]*)["']\s*;`),
			owner: fixed(activities.RemoteImageBoxAttribute),
			drift: "the approval card looks for a box activities.RemoteImageBoxAttribute never marks, so it finds none",
		},
		{
			file: "../frontend/src/features/privacy/privacy.purposes.tsx", name: "PURPOSE_KEY_PATTERN",
			read:  regexp.MustCompile(`\bPURPOSE_KEY_PATTERN\s*=\s*/(.+?)/;`),
			owner: fixed(commsauthz.PurposeKeyPattern.String()),
		},
	}
	sources := map[string]string{}
	for _, limit := range limits {
		t.Run(limit.name, func(t *testing.T) {
			source, read := sources[limit.file]
			if !read {
				source = tsComment.ReplaceAllString(readFile(t, limit.file), " ")
				sources[limit.file] = source
			}
			found := limit.read.FindAllStringSubmatch(source, -1)
			if len(found) != 1 {
				t.Fatalf("%s spells %s %d times, want once: this gate reads a shape that changed", limit.file, limit.name, len(found))
			}
			spelled := found[0][1]
			if tsNumeral.MatchString(spelled) {
				spelled = strings.ReplaceAll(spelled, "_", "")
			}
			for _, want := range limit.owner(t, contract) {
				if spelled != want {
					t.Errorf("%s holds %s = %s, and its owner says %s: %s", limit.file, limit.name, found[0][1], want, limit.drift)
				}
			}
		})
	}
}

// maxLength answers a contract bound the screen copies; a schema node without one fails.
func maxLength(path ...string) func(*testing.T, map[string]any) []string {
	return func(t *testing.T, contract map[string]any) []string {
		t.Helper()
		length, ok := descendContract(t, contract, path...)["maxLength"].(int)
		if !ok {
			t.Fatalf("api/crm.yaml %v declares no maxLength, so the screen's copy has nothing to mirror", path)
		}
		return []string{strconv.Itoa(length)}
	}
}

// bulkConfirmAbove reads the server's confirm line from source, because compose keeps it unexported.
func bulkConfirmAbove(t *testing.T, _ map[string]any) []string {
	t.Helper()
	const declaration = "internal/compose/bulkconfirm.go"
	found := goBulkConfirmAbove.FindStringSubmatch(readFile(t, declaration))
	if found == nil {
		t.Fatalf("%s no longer declares `const bulkConfirmAbove = <rows>` in the shape this gate reads", declaration)
	}
	return []string{found[1]}
}

var goBulkConfirmAbove = regexp.MustCompile(`(?m)^const bulkConfirmAbove\s*=\s*(\d+)\s*$`)

func fixed(value string) func(*testing.T, map[string]any) []string {
	return func(*testing.T, map[string]any) []string { return []string{value} }
}

// fxRateDigits reads the whole and fractional digit counts out of the rate's
// pattern. Both are the one limit the screen holds.
func fxRateDigits(t *testing.T, contract map[string]any) []string {
	t.Helper()
	pattern, ok := bodyProperty(t, contract, "/fx-rates", "post", "rate")["pattern"].(string)
	if !ok {
		t.Fatal("post /fx-rates: rate declares no pattern")
	}
	counts := regexp.MustCompile(`\{1,(\d+)\}`).FindAllStringSubmatch(pattern, -1)
	if len(counts) != 2 {
		t.Fatalf("post /fx-rates: rate pattern %q has %d digit counts, want the whole and the fraction", pattern, len(counts))
	}
	return []string{counts[0][1], counts[1][1]}
}

func bodyProperty(t *testing.T, contract map[string]any, path, method, property string) map[string]any {
	t.Helper()
	operation := descendContract(t, contract, "paths", path, method)
	schema := descendContract(t, operation, "requestBody", "content", "application/json", "schema")
	if ref, ok := schema["$ref"].(string); ok {
		schema = descendContract(t, contract, "components", "schemas", ref[len("#/components/schemas/"):])
	}
	prop, ok := descendContract(t, schema, "properties")[property].(map[string]any)
	if !ok {
		t.Fatalf("%s %s: the body has no %s", method, path, property)
	}
	return prop
}
