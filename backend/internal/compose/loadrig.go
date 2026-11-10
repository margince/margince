// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build loadrig

package compose

import "github.com/margince/margince/backend/internal/modules/capture/gmail"

// PointGoogleCallsAt hands a load-test build's fake Gmail server to the Gmail
// client; the programs read its address at start.
func PointGoogleCallsAt(base string) { gmail.PointGoogleCallsAt(base) }
