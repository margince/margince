// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

import (
	"strings"
	"testing"
)

func TestExternalMetaIsHeldToAKnownShape(t *testing.T) {
	for name, tc := range map[string]struct {
		meta    map[string]string
		refused bool
	}{
		"clean vietnamese value":  {map[string]string{"owner": "Nguyễn Thị Lan", "department": "Nhân sự"}, false},
		"bidi override in value":  {map[string]string{"owner": "Lan‮evil"}, true},
		"bidi isolate in value":   {map[string]string{"owner": "Lan⁦x⁩"}, true},
		"direction mark in value": {map[string]string{"owner": "Lan‏"}, true},
		"newline in value":        {map[string]string{"owner": "Lan\nOwner: CEO"}, true},
		"line separator in value": {map[string]string{"owner": "Lan x"}, true},
		"control char in key":     {map[string]string{"own\x00er": "Lan"}, true},
		"c1 control in value":     {map[string]string{"owner": "Lan\u0085"}, true},
		"folder path":             {map[string]string{"path": "Vận hành cửa hàng/Mở và đóng cửa"}, false},
		"unknown key":             {map[string]string{"salary": "1"}, true},
		"value too long":          {map[string]string{"owner": strings.Repeat("a", 501)}, true},
	} {
		in := ExternalDocument{ExternalID: "id", Meta: tc.meta}
		if err := validateExternal(&in); (err != nil) != tc.refused {
			t.Errorf("%s: err = %v, refused want %v", name, err, tc.refused)
		}
	}
}
