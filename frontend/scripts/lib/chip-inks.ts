// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The inks measured on --bgChip, and the one list two gates read.
// tokens.test.ts pairs each of these with the chip fill and measures it.
// census/rules/chipfill.census.ts fails a rule that paints --bgChip under an
// ink missing here.
//
// One constant rather than one per gate, because the two halves are one
// obligation — a list of measured inks that the tree has outgrown is a gate
// reporting PASS over a case it never looked at.
export const chipInks: readonly string[] = [
  // --textSecondary is absent: no rule paints --bgChip under it.
  // census/rules/chipfill.census.ts fails the day one does, until this list grows.
  "--textPrimary",
  "--tealText",
  "--accentText",
  "--aiText",
];
