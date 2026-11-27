// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

import "github.com/margince/margince/backend/internal/shared/apperrors"

// noBriefYet says why today's brief cannot be read. A reader told only "not
// found" cannot tell a brief not made yet from one it may not read.
const noBriefYet = "No morning brief has been made for today yet. The overnight pass makes one."

// NoBriefYetError answers a read of today's brief before one is made: a 404
// with the reason in words. A client that prefers it hears 204.
type NoBriefYetError struct{}

func (NoBriefYetError) Error() string { return noBriefYet }

func (NoBriefYetError) Unwrap() error { return apperrors.ErrNothingYet }

// MessageFault keeps the wire code a client already branches on.
func (NoBriefYetError) MessageFault() (code, message string) { return "not_found", noBriefYet }
