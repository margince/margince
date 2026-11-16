// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import "github.com/margince/margince/backend/internal/shared/ports/connector"

// Outbound reports whether the mailbox owner wrote this message.
func (m Message) Outbound() bool { return m.direction == connector.DirectionOutbound }
