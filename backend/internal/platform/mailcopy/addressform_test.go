// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailcopy

import (
	"regexp"
	"testing"
)

// Mail to a contact about their own data says quý khách in Vietnamese, as the
// pages its links open do. Bạn is the word for a colleague. The corpus is read
// from the sections that write that mail, so a new line there is checked too.
func TestContactMailAddressesTheReaderAsQuyKhach(t *testing.T) {
	t.Parallel()
	var vietnamese []string
	collect := func(_ func(*Copy) *string, _, _, vi string) { vietnamese = append(vietnamese, vi) }
	confirmLines(collect)
	unsubscribeLines(collect)

	colleague := regexp.MustCompile(`(?i)(^|[^\p{L}])bạn([^\p{L}]|$)`)
	addressed := 0
	for _, line := range vietnamese {
		if colleague.MatchString(line) {
			t.Errorf("contact mail says bạn, the word for a colleague: %q", line)
		}
		if regexp.MustCompile(`(?i)quý khách`).MatchString(line) {
			addressed++
		}
	}
	if addressed == 0 {
		t.Fatal("no contact mail line says quý khách, so this test reads the wrong sections")
	}
}
