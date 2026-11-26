// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryRequiredInquiryBodyIDIsNamedWhenAbsent(t *testing.T) {
	t.Run("recipient_id", func(t *testing.T) {
		_, err := (Assignment{}).plan(context.Background(), ids.ActivityID{}, crmcontracts.WebsiteInquiryAssignmentRequest{})
		faulttest.AssertNamesOmittedID(t, err, "recipient_id")
	})
	t.Run("automation_id", func(t *testing.T) {
		_, _, err := (Settings{}).existingPolicy(context.Background(), ids.Nil, crmcontracts.WebsiteInquiryExistingRequest{})
		faulttest.AssertNamesOmittedID(t, err, "automation_id")
	})
}
