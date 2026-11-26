// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "context"

// SendHandoff brackets the sending RPC. The sender must honor its context;
// credential refresh, prior-send lookup and receipt read-back stay outside it.
type SendHandoff func(context.Context, func(context.Context) (SendReceipt, error)) (SendReceipt, error)

// EmailHandoffSender separates the sending RPC from preparation and read-back.
// Other senders remain supported by guarding their whole SendEmail call.
type EmailHandoffSender interface {
	SendEmailWithHandoff(context.Context, Auth, EmailMessage, SendHandoff) (SendReceipt, error)
}
