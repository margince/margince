// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The most rows one write may set on a contact's addresses and numbers and on
// a company's domains. Each row costs the write its own checks and statements.
// The cap keeps a replace-set inside the request deadline.
const (
	maxContactEmails  = 25
	maxContactPhones  = 25
	maxCompanyDomains = 100
)

// refuseLongerList refuses a list past its cap. A record that already holds a
// longer list may send it back, and may not grow it.
func refuseLongerList(field, rows string, limit, sent, held int) error {
	if sent <= limit || sent <= held {
		return nil
	}
	return httperr.Validation(field, "too_many",
		fmt.Sprintf("%s holds at most %d %s; this one names %d", field, limit, rows, sent))
}
