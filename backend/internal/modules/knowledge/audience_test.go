// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import "testing"

func TestFoldAudience(t *testing.T) {
	for in, want := range map[string]string{
		"Kitchen":            "kitchen",
		"  BẾP  ":            "bep",
		"Đội phục vụ":        "doi phuc vu",
		"đội phục vụ":        "doi phuc vu",
		"Nhân viên cửa hàng": "nhan vien cua hang",
		"":                   "",
	} {
		if got := FoldAudience(in); got != want {
			t.Errorf("FoldAudience(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAudienceMatcher(t *testing.T) {
	m := newAudienceMatcher([]string{"Bếp", " ", "Store Staff"})
	for name, tc := range map[string]struct {
		meta map[string]string
		want bool
	}{
		"same value, no accents typed": {map[string]string{"audience": "bep"}, true},
		"case and accents differ":      {map[string]string{"audience": "  STORE staff "}, true},
		"other audience":               {map[string]string{"audience": "Management"}, false},
		"no audience fails closed":     {map[string]string{"owner": "Lan"}, false},
		"empty audience fails closed":  {map[string]string{"audience": "  "}, false},
		"nil meta fails closed":        {nil, false},
		"partial value is no match":    {map[string]string{"audience": "Store Staff and Management"}, false},
	} {
		if got := m.allows(tc.meta); got != tc.want {
			t.Errorf("%s: allows = %v, want %v", name, got, tc.want)
		}
	}
}

func TestAudienceMatcherIsOffWithoutValues(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"", "  "}} {
		if m := newAudienceMatcher(in); m != nil {
			t.Errorf("newAudienceMatcher(%q) = %v, want nil (no filter)", in, m)
		}
	}
}
