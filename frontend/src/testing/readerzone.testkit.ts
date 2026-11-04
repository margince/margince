// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { vi } from "vitest";

/**
 * Pretends the browser runs in `zone`, for a suite that mounts a screen with
 * no reader-zone provider. Such a screen reads the browser's zone, so without
 * this the days it prints would move with the machine the suite runs on. The
 * formatters take their zone as an argument and never read this, so only the
 * screen's own lookup moves.
 */
export function pretendReaderZone(zone: string): void {
  const real = Intl.DateTimeFormat().resolvedOptions();
  vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
    ...real,
    timeZone: zone,
  });
}
