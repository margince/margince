/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { STATUS_KINDS, StatusBadge, statusTone } from "./statusbadge";

afterEach(cleanup);

function badgeFor(label: string): HTMLElement {
  const badge = screen.getByText(label).closest(".badge");
  if (!(badge instanceof HTMLElement)) {
    throw new Error(`no .badge carries "${label}"`);
  }
  return badge;
}

describe("StatusBadge", () => {
  it("maps each kind to the one tone the palette gives it", () => {
    expect(STATUS_KINDS.map(statusTone)).toEqual([
      "success",
      "default",
      "warning",
      "danger",
    ]);
  });

  it("draws neutral as the plain grey badge", () => {
    render(<StatusBadge kind="neutral" label="Lost" />);
    expect(badgeFor("Lost").className).toBe("badge");
  });

  it("draws each other kind in its tone", () => {
    render(<StatusBadge kind="failure" label="Failed" />);
    expect(badgeFor("Failed")).toHaveClass("badge-danger");
  });

  it("puts the live dot first when the status is running", () => {
    render(<StatusBadge kind="neutral" label="Syncing" live />);
    const first = badgeFor("Syncing").firstElementChild;
    expect(first).toHaveClass("badge-live-dot");
  });

  it("draws no dot when the status is not live", () => {
    render(<StatusBadge kind="done" label="Connected" />);
    expect(badgeFor("Connected").querySelector(".badge-live-dot")).toBeNull();
  });
});
