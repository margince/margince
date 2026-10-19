// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { PageZonesShape } from "./pagezones";

// The layout choices RecordView makes once per record: where its verbs land,
// how large its mark is, and which columns its grid has.

// Where a record's verbs land. Three places can hold them — beside the
// standing column, on the identity's own row, or in a band under the header —
// and the choice is made once here rather than restated as a condition at
// each of the three, where a reader had to hold all three at once to know
// which one wins.
export function actionsPlacement(
  actions: ReactNode,
  inline: boolean | undefined,
  controls: ReactNode,
): "none" | "inline" | "controls" | "below" {
  if (!actions) {
    return "none";
  }
  if (inline) {
    return "inline";
  }
  return controls ? "controls" : "below";
}

// The mark's rung follows the head's: the masthead draws it at the record
// rung, the compact head one under, and the narrow head keeps the list's.
export function headMarkSize(
  wide: boolean,
  compact: boolean,
): "md" | "lg" | "xl" {
  if (compact) {
    return "xl";
  }
  return wide ? "xl" : "md";
}

// Which columns this record actually has. The grid itself is `PageZones` — a
// record page is one page shape among others, and the ratios and the folds are
// not the record's to own.
export function zonesShape(
  hasRail: boolean,
  hasAside: boolean,
): PageZonesShape {
  if (hasRail && hasAside) {
    return "both";
  }
  if (hasRail) {
    return "rail";
  }
  if (hasAside) {
    return "aside";
  }
  return "single";
}

// What the record adds to the grid container on top of the layout.
//
// `arrive-stack` on every shape, including the one with no columns: a record's
// blocks arrive individually (design-system/enter.css), and a container that is
// a stack does not itself arrive. Leaving one link of that chain unmarked is
// what makes a block fade in BEHIND a parent that is still fading in — two
// fades multiplied, which reads as the content being dim rather than as it
// arriving.
//
// `record-zones` carries only the phone bottom clearance for the sticky action
// bar (composed.css), which is why the single-column shape does not get it: a
// record with no columns never had it either.
export function zonesClassName(shape: PageZonesShape): string {
  return shape === "single" ? "arrive-stack" : "record-zones arrive-stack";
}
