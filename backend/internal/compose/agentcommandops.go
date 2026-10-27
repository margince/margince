// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The action-shaped update operations, named once so restCommands
// (agentcommand.go) and the gate spell each operationId the same way.

const (
	opApplyTag            = "applyTag"
	opRemoveTag           = "removeTag"
	opAddOfferLineItem    = "addOfferLineItem"
	opUpdateOfferLineItem = "updateOfferLineItem"
	opRemoveOfferLineItem = "removeOfferLineItem"
)
