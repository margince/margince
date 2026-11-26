// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/compose/securitytxt"

// SecurityTxt is the RFC 9116 document configured for the API.
type SecurityTxt = securitytxt.Document

// WebSecurityTxt is the config setting for that document.
var WebSecurityTxt = securitytxt.Setting
