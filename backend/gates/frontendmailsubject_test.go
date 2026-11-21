// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server both group a thread by its subject without the
// reply and forward markers. Two lists would fold different mail into one
// thread. So the TypeScript list mirrors the Go one, compared both ways.

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

const frontendMailSubject = "../frontend/src/format/mailsubject.ts"

var (
	tsMailSubjectList = regexp.MustCompile(`(?s)MAIL_SUBJECT_PREFIXES: readonly string\[\] = \[(.*?)\];`)
	tsQuotedWord      = regexp.MustCompile(`["']([^"']*)["']`)
)

func TestTheFrontendSubjectPrefixesMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendMailSubject)
	if err != nil {
		t.Fatalf("reading the frontend subject prefixes: %v", err)
	}
	list := tsMailSubjectList.FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares MAIL_SUBJECT_PREFIXES as an array literal", frontendMailSubject)
	}
	var inTS []string
	for _, m := range tsQuotedWord.FindAllSubmatch(tsComment.ReplaceAll(list[1], nil), -1) {
		inTS = append(inTS, string(m[1]))
	}
	inGo := mailsubject.PrefixWords()
	slices.Sort(inTS)
	slices.Sort(inGo)
	if len(inTS) == 0 || !slices.Equal(inTS, inGo) {
		t.Errorf("%s lists %v, and mailsubject.PrefixWords lists %v: change both", frontendMailSubject, inTS, inGo)
	}
}
