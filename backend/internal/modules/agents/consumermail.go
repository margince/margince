// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "context"

// ConsumerMail answers whether a mail domain is a consumer mailbox provider
// rather than a company's own domain — a "yes" means no employer may be
// derived from an address at it.
//
// It is an interface rather than a value because the answer is not static:
// the pinned baseline is only half of it, and the other half is a table an
// operator administers per workspace. Two doors ask this question — capture's
// tier ladder and this tool — and an operator who marks a host consumer at one
// of them has marked it at both, or the two doors create different companies
// from the same address. compose implements it over `platform/freemail`; this
// module never reads storage directly.
type ConsumerMail interface {
	IsConsumer(ctx context.Context, domain string) (bool, error)
}
