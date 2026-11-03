// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	lifecycleScopeUnknown    = string(crmcontracts.SignalEvidenceLifecycleScopeUnknown)
	lifecycleScopeCompany    = string(crmcontracts.SignalEvidenceLifecycleScopeCompany)
	lifecycleScopeWorkstream = string(crmcontracts.SignalEvidenceLifecycleScopeWorkstream)
)

// validateLifecycleEvidence judges scope only on a contract ending. On any other
// kind the scope means nothing and eventSignalEvidence never records it, so a
// stray value is dropped rather than refusing the whole reply.
func validateLifecycleEvidence(event extractedEvent, thread settledThread) string {
	if event.Kind != signalKindContractEnded {
		return ""
	}
	switch event.LifecycleScope {
	case "", lifecycleScopeUnknown:
		if event.Workstream != "" {
			return "an unknown lifecycle scope cannot name an engagement"
		}
		return ""
	case lifecycleScopeCompany, lifecycleScopeWorkstream:
	default:
		return "lifecycle scope must be company, workstream or unknown"
	}
	if strings.TrimSpace(event.Quote) == "" {
		return "a scoped contract ending must quote its source"
	}
	for _, message := range thread.Messages {
		if message.ID.String() == event.MessageID {
			body := textlang.CurrentMessage(message.Body)
			if !values.Quoted(body, event.Quote) {
				return "lifecycle scope quotes words absent from the current message"
			}
			if event.Workstream != "" && !values.Quoted(body, event.Workstream) {
				return "the named workstream is absent from the current message"
			}
			return ""
		}
	}
	return "lifecycle scope has no supplied source"
}

func eventSignalEvidence(event extractedEvent, cited ids.UUID, thread settledThread) signals.DerivedEvidence {
	evidence := signals.DerivedEvidence{ActivityID: cited, Snippet: event.Summary}

	if event.Kind == signalKindContractEnded {
		evidence.LifecycleScope, evidence.Workstream = event.LifecycleScope, event.Workstream
		for _, message := range thread.Messages {
			if message.ID != cited {
				continue
			}
			if proof, ok := textlang.ProveSenderQuote(message.Body, event.Quote); ok {
				evidence.SourceQuote = &proof
			}
		}
	}
	return evidence
}
