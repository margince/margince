// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./readingsfloor.css";

// The caveat that qualifies a WHOLE row of readings: a source read to its
// limit, making every figure above it a floor rather than a count.
//
// It belongs to the row and not to a slot, and that is the whole reason this
// is one component rather than a prop each row shape spells for itself. A row
// of readings is read as one statement, so a caveat pinned to one figure
// invites the reading where the others are exact — and `StatStrip` and
// `ReadingsGrid` are two shapes of one row, which would have meant two places
// to get that wrong.
export function ReadingsFloor({ children }: Readonly<{ children: ReactNode }>) {
  return <p className="readings-floor">{children}</p>;
}
