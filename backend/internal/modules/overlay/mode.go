// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// The installation's system-of-record mode: which store answers a read.
//
// It lived on the workspace row as x_sor_mode until ADR-0091 §1 moved it into
// overlay_mode, a single row this module owns outright. The two values below
// are what overlay_mode_sor_mode_check admits.
//
// Named because the mode is read in three places and written in three more, and
// a typo in any of them reads as "this installation is native" — which is not a
// failure but a silent wrong answer: reads go to first-party tables for an
// installation whose records live in the incumbent.
const (
	modeOverlay = "overlay"
	modeNative  = "native"
)
