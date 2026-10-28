// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ControllerSendSQL reads, off the delivery under alias `delivery`, whether the
// installation's own relay sent it and the From it records.
const ControllerSendSQL = `coalesce(delivery.sender_kind = 'controller', false), delivery.from_address`

// ControllerSendOf is the wire mark of a mail sent through the installation's
// own relay, and nil for every other row.
func ControllerSendOf(controller bool, from *string) *crmcontracts.ControllerSend {
	if !controller {
		return nil
	}
	return &crmcontracts.ControllerSend{FromAddress: from}
}
