// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";
import type { ButtonVariant } from "./button";

// A screen, panel or card in hand has at most one filled control. A row in a
// list has none. One fill per row reads as a column of loud buttons, and their
// colours change row by row.
const QuietContext = createContext(false);

/**
 * Marks a region whose controls draw outlined. A row in a list wraps its verbs
 * in this. A `primary` or `ai` verb inside, however deep, keeps its job and
 * its ✦ and gives up the fill.
 */
export function QuietVerbs({
  quiet = true,
  children,
}: Readonly<{ quiet?: boolean; children: ReactNode }>) {
  return (
    <QuietContext.Provider value={quiet}>{children}</QuietContext.Provider>
  );
}

/** Whether the control being drawn sits inside a `QuietVerbs` region. */
export function useQuietVerbs(): boolean {
  return useContext(QuietContext);
}

/**
 * The variant a control draws at its region's volume. Only the two fills step
 * down; `danger` keeps its red, because a quiet destructive verb is a trap.
 */
export function atRegionVolume<V extends ButtonVariant | "bare">(
  variant: V,
  quiet: boolean,
): V | "ghost" | "aiQuiet" {
  if (!quiet) return variant;
  if (variant === "primary") return "ghost";
  if (variant === "ai") return "aiQuiet";
  return variant;
}
