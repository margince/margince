// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func TestAClaimStoresUTCForAZoneTheServerDoesNotKnow(t *testing.T) {
	errProbe := errors.New("the database did not answer")
	knows := func(known bool, err error) func(string) (bool, error) {
		return func(string) (bool, error) { return known, err }
	}
	for name, tc := range map[string]struct {
		sent    string
		probe   func(string) (bool, error)
		want    string
		wantErr error
	}{
		"a zone both engines know is kept":        {sent: "Europe/Berlin", probe: knows(true, nil), want: "Europe/Berlin"},
		"a zone Go does not know reads as UTC":    {sent: "Mars/Olympus_Mons", probe: knows(true, nil), want: storekit.FallbackZone},
		"a zone the database lacks reads as UTC":  {sent: "Europe/Berlin", probe: knows(false, nil), want: storekit.FallbackZone},
		"a malformed name is left for the parser": {sent: "Local", probe: knows(true, nil), want: "Local"},
		"an empty name is left for the default":   {sent: "", probe: knows(true, nil), want: ""},
		"a failed probe is returned":              {sent: "Europe/Berlin", probe: knows(false, errProbe), want: "Europe/Berlin", wantErr: errProbe},
	} {
		got, err := claimedZone(tc.sent, tc.probe)
		if got != tc.want || !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: claimedZone(%q) = %q, %v; want %q, %v", name, tc.sent, got, err, tc.want, tc.wantErr)
		}
	}
}
