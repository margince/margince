// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";

type ChipLook = "chip-dense" | "chip-lead" | "chip-wrap";

/** The chip's classes, for a control that opens something from a chip's face
 *  (`Popover`'s trigger, `OverflowMenu face="chip"`) and so cannot be a `Chip`.
 *  Keyed by class, as `clsx` is, so the class census reads each name. */
export function chipClass(looks: Readonly<Partial<Record<ChipLook, boolean>>>) {
  const on = Object.entries(looks).filter(([, wanted]) => wanted);
  return ["chip", ...on.map(([look]) => look)].join(" ");
}

/** What a chip draws inside its edge: its mark, then its words. */
export function ChipFace({
  mark,
  children,
}: Readonly<{ mark: ReactNode; children: ReactNode }>) {
  return (
    <>
      {mark}
      <span className="chip-label">{children}</span>
    </>
  );
}
