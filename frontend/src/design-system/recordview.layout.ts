// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { PageZonesShape } from "./pagezones";

// The layout choices RecordView makes once per record: where its verbs land,
// how large its mark is, and which columns its grid has.

// Where a record's verbs land: beside the standing column, on the identity's
// own row, or in a band under the head. The choice is made once, here.
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

// The mark's size follows the head's: large on a masthead or a compact head,
// and the list's size on a narrow one.
export function headMarkSize(
  wide: boolean,
  compact: boolean,
): "md" | "lg" | "xl" {
  if (compact) {
    return "xl";
  }
  return wide ? "xl" : "md";
}

// Which columns this record has. The grid itself is `PageZones`, because the
// ratios and folds belong to every page shape, not to the record.
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

// `arrive-stack` on every shape: a record's blocks arrive one by one
// (enter.css), and an unmarked parent would fade in under them, two fades.
// `record-zones` carries only the phone clearance for the sticky verb bar,
// which a record with no columns does not have.
export function zonesClassName(shape: PageZonesShape): string {
  return shape === "single" ? "arrive-stack" : "record-zones arrive-stack";
}
