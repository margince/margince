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
  vi.spyOn(nativeFormatterPrototype(), "resolvedOptions").mockReturnValue({
    ...real,
    timeZone: zone,
  });
}

// Fake timers swap in an `Intl.DateTimeFormat` whose formatters bind the
// native prototype's methods, and whose own prototype only inherits them. A
// spy must sit where `resolvedOptions` is defined, whichever is installed.
function nativeFormatterPrototype(): Intl.DateTimeFormat {
  let proto: Intl.DateTimeFormat = Intl.DateTimeFormat.prototype;
  while (!Object.hasOwn(proto, "resolvedOptions")) {
    proto = Object.getPrototypeOf(proto);
  }
  return proto;
}
