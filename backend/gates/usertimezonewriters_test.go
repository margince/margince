// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A user's time zone has one writer after their row exists: writeTimezoneTx
// in identity/usertimezone.go. It is the one place that keeps a chosen zone
// from being replaced by a browser guess, and that sets timezone_chosen with
// the zone. A second writer would store a zone without saying who picked it.
//
// TestEachFactHasOneWriter (onewriters_test.go) holds it with the matcher
// below. It matches an UPDATE or INSERT of app_user that assigns timezone or
// timezone_chosen, in its SET or column list, in any letter case.

import (
	"regexp"
	"testing"
)

var (
	appUserUpdateSet = regexp.MustCompile(`(?is)\bupdate\s+app_user\b.*?\bset\b(.*?)(?:\bwhere\b|\breturning\b|\bfrom\b|$)`)
	appUserInsert    = regexp.MustCompile(`(?is)\binsert\s+into\s+app_user\s*\(([^)]*)\)`)
	timezoneAssigned = regexp.MustCompile(`(?i)(?:^|[\s,(])timezone(?:_chosen)?\s*=`)
	timezoneColumn   = regexp.MustCompile(`(?i)(?:^|[\s,])timezone(?:_chosen)?\s*(?:,|$)`)
)

// assignsTimezone reports whether one SQL statement writes a user's zone.
func assignsTimezone(sql string) bool {
	for _, m := range appUserUpdateSet.FindAllStringSubmatch(sql, -1) {
		if timezoneAssigned.MatchString(m[1]) {
			return true
		}
	}
	for _, m := range appUserInsert.FindAllStringSubmatch(sql, -1) {
		if timezoneColumn.MatchString(m[1]) {
			return true
		}
	}
	return false
}

// The matcher sees an assignment anywhere in the SET list, in any case. It
// sees an INSERT that names the column, and it does not see a read.
func TestTheTimezoneWriterMatcherSeesEveryShape(t *testing.T) {
	t.Parallel()
	for sql, want := range map[string]bool{
		`UPDATE app_user SET timezone = $2 WHERE id = $1`:                                  true,
		`UPDATE app_user SET updated_at = now(), timezone = $2 WHERE id = $1`:              true,
		`update app_user set work_days = $3, Timezone_Chosen = true where id = $1`:         true,
		"UPDATE app_user\n   SET display_name = $2,\n       timezone = $3\n WHERE id = $1": true,
		`INSERT INTO app_user (email, display_name, timezone) VALUES ($1, $2, $3)`:         true,
		`UPDATE app_user SET work_days = $2 WHERE id = $1 AND timezone = 'UTC'`:            false,
		`SELECT timezone, timezone_chosen FROM app_user WHERE id = $1`:                     false,
		`UPDATE app_user SET work_timezone_hint = $2 WHERE id = $1`:                        false,
	} {
		if got := assignsTimezone(sql); got != want {
			t.Errorf("assignsTimezone(%q) = %v, want %v", sql, got, want)
		}
	}
}
