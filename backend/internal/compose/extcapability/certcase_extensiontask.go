// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extcapability

// The certification seam for an extension's AI task. The core writes no prompt
// here, because the unit supplies the system text and the messages. Every unit's
// call goes through Request. CertRequest is the one request the certification
// layer builds with it. That lets the prompt gate see that Request is reached.

import (
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// CertRequest builds the request a certification case sends for a unit's task,
// with a fixed instruction standing for the one a unit writes.
func CertRequest(messages []extension.Message) (model.Request, error) {
	return Request(extension.CompleteRequest{
		System:        "You summarise the note you are given for a sales rep, in one sentence.",
		Messages:      messages,
		NoContactData: "certification corpus",
	})
}
