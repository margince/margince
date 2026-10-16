// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The ids in reading order, absent ones dropped; `undefined` when none is left,
// so React writes no `aria-describedby` rather than an empty one.
export function describedByIds(
  ...ids: readonly (string | false | null | undefined)[]
): string | undefined {
  return ids.filter(Boolean).join(" ") || undefined;
}
