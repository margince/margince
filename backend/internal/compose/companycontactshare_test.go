// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The change_access tool is served by the REST route, so both reach
// ShareCompanyContacts. It refuses an omitted subject before it opens a
// transaction, so the nil database is never reached.
func TestAnOmittedShareSubjectIsNamed(t *testing.T) {
	_, err := ShareCompanyContacts(context.Background(), nil, identity.NewService(nil),
		ids.From[ids.CompanyKind](ids.NewV7()), crmcontracts.ShareCompanyContactsRequest{
			SubjectType: "user", Access: "read",
		})
	faulttest.AssertNamesOmittedID(t, err, "subject_id")
}
