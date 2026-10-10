// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"regexp"
	"strings"
)

// updatesActivity and nullsBody read the two halves of the act, normalised.
//
// Substring matching on `update activity` and `body = null` was too literal to
// be a census: `UPDATE "activity"`, `UPDATE public.activity`, `UPDATE only
// activity`, an alias after the target, or `body=NULL` with no spaces all
// destroy the same text. Under-recognition is the direction a census of
// destroyers must not fail in.
var (
	updatesActivity = regexp.MustCompile(`(?s)\bupdate\s+(?:only\s+)?(?:"?[a-z_][a-z_0-9]*"?\s*\.\s*)?"?activity"?(?:\s|$)`)
	nullsBody       = regexp.MustCompile(`(?s)\b(?:"?[a-z_][a-z_0-9]*"?\s*\.\s*)?"?body"?\s*=\s*null\b`)
)

// DestroysActivityBody reports whether a SQL statement clears an activity's
// text. The privacy census and the gate on arms outside privacy both read
// statements through it.
func DestroysActivityBody(statement string) bool {
	low := strings.ToLower(statement)
	return updatesActivity.MatchString(low) && nullsBody.MatchString(low)
}
