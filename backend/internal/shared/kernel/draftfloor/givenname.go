// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import "strings"

// vietnameseFamilyNames are the common Vietnamese family names, lower-cased,
// each in its Vietnamese and its plain ASCII spelling. A Vietnamese name puts
// the family name first and the given name last. The screen keeps the same
// list in frontend/src/format/greetingname.ts, held by
// backend/gates/frontendgivenname_test.go.
var vietnameseFamilyNames = setOfNames(
	"nguyễn", "nguyen", "trần", "tran", "lê", "le", "phạm", "pham", "hoàng", "hoang",
	"huỳnh", "huynh", "phan", "vũ", "vu", "võ", "vo", "đặng", "dang", "bùi", "bui",
	"đỗ", "do", "hồ", "ho", "ngô", "ngo", "dương", "duong", "lý", "ly", "đinh", "dinh",
	"trương", "truong", "đào", "dao", "đoàn", "doan", "trịnh", "trinh", "lương", "luong",
	"cao", "chu", "quách", "quach", "tăng", "tang", "triệu", "trieu", "kiều", "kieu",
	"vương", "vuong", "lưu", "luu", "phùng", "phung", "lâm", "lam", "tạ", "ta",
)

func setOfNames(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}

// VietnameseFamilyNames lists the family names, for the parity gate.
func VietnameseFamilyNames() []string {
	out := make([]string, 0, len(vietnameseFamilyNames))
	for name := range vietnameseFamilyNames {
		out = append(out, name)
	}
	return out
}

// GivenName is the word of a display name a greeting uses. That is the last
// word when the name starts with a Vietnamese family name, else the first.
// A name that also ends with one is written in Western order already.
func GivenName(display string) string {
	fields := strings.Fields(OneLine(display))
	switch {
	case len(fields) == 0:
		return ""
	case len(fields) > 1 && familyName(fields[0]) && !familyName(fields[len(fields)-1]):
		return fields[len(fields)-1]
	default:
		return fields[0]
	}
}

func familyName(word string) bool { return vietnameseFamilyNames[strings.ToLower(word)] }
