// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { company360 } from "../company.fixtures";
import { RecordSpine } from "./spine";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The layout a browser would measure: an axis holding `held` pixels in `shown`.
function laidOut(held: number, shown: number) {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(held);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(shown);
}

function drawSpine() {
  const { container } = render(
    <LocaleProvider initial="en">
      <RecordSpine
        source={{
          ...company360,
          as_of: "2026-08-25T09:00:00Z",
          last_outbound_at: "2026-08-18T09:00:00Z",
        }}
      />
    </LocaleProvider>,
  );
  const axis = container.querySelector<HTMLElement>(".co-spine-scroll");
  if (axis === null) throw new Error("the spine drew no axis");
  return axis;
}

describe("the spine's axis", () => {
  it("is a named tab stop while it holds more than it shows", () => {
    laidOut(900, 252);
    const axis = drawSpine();
    expect(screen.getByRole("region", { name: en["record.timeline"] })).toBe(
      axis,
    );
    expect(axis.tabIndex).toBe(0);
  });

  it("takes no role and no tab stop while it fits", () => {
    laidOut(252, 252);
    const axis = drawSpine();
    expect(axis.getAttribute("role")).toBeNull();
    expect(axis.hasAttribute("tabindex")).toBe(false);
  });
});
