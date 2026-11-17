// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// No shipped UI string says that granting the send permission approves a
// message. An unattended run holds that permission and still stages every send.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// permissionIsApprovalClaims are the shapes this claim shipped in. A language
// needs its own phrasing, so the patterns cannot be derived from the code.
var permissionIsApprovalClaims = map[string][]*regexp.Regexp{
	"en": {
		regexp.MustCompile(`(granting the permission|giving that passport) is the approval`),
		regexp.MustCompile(`send anything or change a record without your approval`),
	},
	"de": {
		regexp.MustCompile(`mit (der berechtigung|diesem passport) ist die freigabe bereits erteilt`),
		regexp.MustCompile(`ohne deine freigabe etwas senden oder einen datensatz ändern`),
	},
	"vi": {
		regexp.MustCompile(`cấp (quyền|passport đó) chính là phê duyệt`),
		regexp.MustCompile(`gửi bất cứ thứ gì hay sửa hồ sơ khi bạn chưa duyệt`),
	},
}

func TestNoLocaleClaimsTheSendPermissionIsTheApproval(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot, "frontend", "src", "i18n")
	for _, locale := range shippedLocales(t, dir) {
		claims, covered := permissionIsApprovalClaims[locale]
		if !covered {
			t.Errorf("frontend/src/i18n/%s.ts ships to users but this gate knows no %s phrasing of \"the permission is the approval\". Add the shapes to permissionIsApprovalClaims", locale, locale)
			continue
		}
		raw, err := gatekit.ReadFile(filepath.Join(dir, locale+".ts"))
		if err != nil {
			t.Fatalf("reading the %s locale: %v", locale, err)
		}
		body := strings.ToLower(shippedCopy(string(raw)))
		for _, claim := range claims {
			if loc := claim.FindStringIndex(body); loc != nil {
				t.Errorf("frontend/src/i18n/%s.ts promises %q, but an unattended run stages its sends whatever permission it holds", locale, strings.TrimSpace(body[loc[0]:loc[1]]))
			}
		}
	}
}
