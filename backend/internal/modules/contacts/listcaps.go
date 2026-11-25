// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The most rows a contact's address and number lists, and a company's domain
// list, take from one write. Each row costs the write its own checks and
// statements, so the cap is what keeps a replace-set inside the request deadline.
const (
	maxContactEmails  = 25
	maxContactPhones  = 25
	maxCompanyDomains = 100
)

// refuseLongerList refuses a list past its cap, unless the record already holds
// at least as many rows: a stored list past the cap may be sent back, and not grown.
func refuseLongerList(field, rows string, limit, sent, held int) error {
	if sent <= limit || sent <= held {
		return nil
	}
	return httperr.Validation(field, "too_many",
		fmt.Sprintf("%s holds at most %d %s; this one names %d", field, limit, rows, sent))
}
