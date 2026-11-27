// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server both group a thread by its subject without the
// reply and forward markers. Two lists would fold different mail into one
// thread. So the TypeScript list mirrors the Go one, compared both ways. The
// forward list alone is mirrored too: both splitters read it from the subject.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

const frontendMailSubject = "../frontend/src/format/mailsubject.ts"

var (
	tsMailSubjectList = regexp.MustCompile(`(?s)MAIL_SUBJECT_PREFIXES: readonly string\[\] = \[(.*?)\];`)
	tsMailForwardList = regexp.MustCompile(`(?s)MAIL_FORWARD_PREFIXES: readonly string\[\] = \[(.*?)\];`)
	tsQuotedWord      = regexp.MustCompile(`["']([^"']*)["']`)
)

func TestTheFrontendSubjectPrefixesMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendMailSubject)
	if err != nil {
		t.Fatalf("reading the frontend subject prefixes: %v", err)
	}
	var forwards []string
	for _, prefix := range mailsubject.ForwardPrefixes() {
		forwards = append(forwards, strings.TrimSuffix(prefix, ":"))
	}
	for name, mirror := range map[string]struct {
		list *regexp.Regexp
		inGo []string
	}{
		"MAIL_SUBJECT_PREFIXES": {tsMailSubjectList, mailsubject.PrefixWords()},
		"MAIL_FORWARD_PREFIXES": {tsMailForwardList, forwards},
	} {
		list := mirror.list.FindSubmatch(source)
		if list == nil {
			t.Fatalf("%s no longer declares %s as an array literal", frontendMailSubject, name)
		}
		var inTS []string
		for _, m := range tsQuotedWord.FindAllSubmatch([]byte(gatekit.TSWithoutComments(string(list[1]))), -1) {
			inTS = append(inTS, string(m[1]))
		}
		slices.Sort(inTS)
		slices.Sort(mirror.inGo)
		if len(inTS) == 0 || !slices.Equal(inTS, mirror.inGo) {
			t.Errorf("%s lists %v as %s, and Go lists %v: change both", frontendMailSubject, inTS, name, mirror.inGo)
		}
	}
}
