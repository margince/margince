// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// SendProbe answers whether the principal in ctx has a mailbox that can
// transmit. Composition injects the check the send itself runs, so the card and
// the send ask the same question.
type SendProbe func(ctx context.Context) (capable bool, err error)

// WithSendProbe lets pending mail proposals say that approving could only be refused.
func (s *Service) WithSendProbe(probe SendProbe) *Service {
	s.sendProbe = probe
	return s
}

// readSendBlockers marks each pending mail proposal whose release has no mailbox
// to send from. The question is asked as the principal the release runs as. An
// agent's staging runs as that agent for its user. A held draft runs as its decider.
//
// A probe that cannot answer marks nothing. The send's own refusal still
// stands. A card blocked on a failed question would strand a sendable mail.
func (s *Service) readSendBlockers(ctx context.Context, rows []crmcontracts.Approval, subjects []row) {
	if s.sendProbe == nil {
		return
	}
	for i := range rows {
		if rows[i].Status != crmcontracts.ApprovalStatusPending || !mailKinds[subjects[i].Kind] {
			continue
		}
		asSender, _, err := EffectContext(s.withPreviewEffect(ctx, subjects[i]))
		if err != nil {
			continue
		}
		if capable, err := s.sendProbe(asSender); err == nil && !capable {
			rows[i].MailboxSendCapable = new(false)
		}
	}
}
