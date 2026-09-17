// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The inks measured ON --bgChip, and the ONE list two gates read: tokens.test.ts
// pairs each of these with the chip fill and measures it, and chipfill.test.ts
// fails a rule that paints --bgChip under an ink missing here.
//
// One constant rather than one per gate, because the two halves are one
// obligation — a list
// of measured inks that the tree has outgrown is a gate reporting PASS over a
// case it never looked at.
export const chipInks: readonly string[] = [
  "--textChip",
  "--textPrimary",
  "--textContent",
  "--tealText",
  "--accentText",
  "--aiText",
];
