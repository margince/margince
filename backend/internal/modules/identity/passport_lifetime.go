// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"fmt"
	"time"
	"unicode/utf8"
)

// admitCallerChosenFields refuses what IssuePassportRequest forbids on the
// wire. An OAuth-minted passport derives its label from the client id and is
// not caller-chosen, so only the local doors run this.
func admitCallerChosenFields(in IssuePassportInput) error {
	if in.Label != nil && utf8.RuneCountInString(*in.Label) > maxPassportLabelRunes {
		return &InvalidPassportFieldError{Field: "label", Code: "label_too_long", Message: fmt.Sprintf("must be at most %d characters", maxPassportLabelRunes)}
	}
	if in.TTL != nil && (*in.TTL <= 0 || *in.TTL > maxPassportTTL) {
		return &InvalidPassportFieldError{Field: "ttl_hours", Code: "ttl_out_of_range", Message: fmt.Sprintf("must be 1 to %d hours", int(maxPassportTTL/time.Hour))}
	}
	return nil
}

// lifetimeOf is how long a passport lives, nil for one that never expires.
// A connection's passport always expires: its refresh is what renews it, and the
// connection readers compare against its expiry.
func lifetimeOf(in IssuePassportInput, grantBound bool) (*time.Duration, error) {
	if in.NeverExpires {
		switch {
		case grantBound:
			return nil, &InvalidPassportFieldError{Field: "never_expires", Code: "never_expires_on_connection", Message: "a connection's passport always expires; mint a passport by hand to have one that does not"}
		case in.TTL != nil:
			return nil, &InvalidPassportFieldError{Field: "never_expires", Code: "never_expires_with_ttl", Message: "choose either a lifetime in hours or never_expires, not both"}
		}
		return nil, nil
	}
	ttl := defaultPassportTTL
	if in.TTL != nil {
		ttl = *in.TTL
	}
	if ttl <= 0 || ttl > maxPassportTTL {
		return nil, fmt.Errorf("passport lifetime %s is outside 0 to %s", ttl, maxPassportTTL)
	}
	return &ttl, nil
}
