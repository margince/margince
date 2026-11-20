// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { vi } from "vitest";

/**
 * Text that fits its width and is held to `lines` by a clamp, on every element:
 * happy-dom lays nothing out and drops `-webkit-line-clamp` from computed style.
 */
export function stubLineClamp({
  lines,
  scroll,
  client,
}: Readonly<{ lines: string; scroll: number; client: number }>): void {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(220);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(220);
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(
    scroll,
  );
  vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockReturnValue(
    client,
  );
  const read = CSSStyleDeclaration.prototype.getPropertyValue;
  vi.spyOn(
    CSSStyleDeclaration.prototype,
    "getPropertyValue",
  ).mockImplementation(function (this: CSSStyleDeclaration, name: string) {
    return name === "-webkit-line-clamp" ? lines : read.call(this, name);
  });
}
