/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { PHONE_MAX_WIDTH } from "../lib/viewport";
import { stubWindowWidth } from "../testing/phonerow.testkit";
import { Pager } from "./pager";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Page `current` of a counted list of 1,204 rows, 25 to a page.
function renderPager(current: number) {
  return render(
    <LocaleProvider initial="en">
      <Pager
        current={current}
        lastPage={49}
        hasMore
        endKnown
        perPage={25}
        range={{
          first: (current - 1) * 25 + 1,
          last: Math.min(current * 25, 1204),
          total: 1204,
        }}
        onGoto={() => undefined}
        onPerPage={() => undefined}
      />
    </LocaleProvider>,
  );
}

const strip = (container: HTMLElement) =>
  [...container.querySelectorAll(".lt-pager > *")].map(
    (slot) => slot.textContent || slot.getAttribute("aria-label"),
  );

describe("the pager on a computer", () => {
  it.each([1024, 1440])("draws seven slots with words at %ipx", (width) => {
    stubWindowWidth(width);
    const { container } = renderPager(5);
    expect(strip(container)).toHaveLength(9);
    expect(screen.getByRole("button", { name: "Next" })).toHaveTextContent(
      "Next",
    );
  });

  it("leads its row with the range, then the strip with words either side", () => {
    stubWindowWidth(1440);
    const { container } = renderPager(5);

    const row = container.querySelector(".lt-footrow");
    expect(row?.firstElementChild).toHaveClass("lt-range");
    expect(row?.firstElementChild).toHaveTextContent("101-125 of 1,204");
    expect(strip(container)).toEqual([
      "Previous",
      "1",
      "…",
      "4",
      "5",
      "6",
      "…",
      "49",
      "Next",
    ]);
    expect(
      screen.getByRole("combobox", { name: "Rows per page" }),
    ).toHaveTextContent("25 per page");
  });
});

describe("the pager on a tablet", () => {
  it.each([768, 900])("draws five slots between chevrons at %ipx", (width) => {
    stubWindowWidth(width);
    const { container } = renderPager(5);
    expect(strip(container)).toEqual([
      "Previous",
      "1",
      "…",
      "5",
      "…",
      "49",
      "Next",
    ]);
    expect(
      screen.getByRole("button", { name: "Previous" }).querySelector("svg"),
    ).not.toBeNull();
    expect(
      screen.getByRole("combobox", { name: "Rows per page" }),
    ).toHaveTextContent("25 per page");
  });
});

describe("the pager on a phone", () => {
  it("draws five slots between chevrons named Previous and Next", () => {
    stubWindowWidth(PHONE_MAX_WIDTH);
    const { container } = renderPager(5);

    expect(strip(container)).toEqual([
      "Previous",
      "1",
      "…",
      "5",
      "…",
      "49",
      "Next",
    ]);
    for (const name of ["Previous", "Next"]) {
      const step = screen.getByRole("button", { name });
      expect(step.querySelector("svg")).not.toBeNull();
      expect(step).toHaveTextContent("");
    }
  });

  it("keeps the range, and names its compact page size in full", () => {
    stubWindowWidth(390);
    const { container } = renderPager(1);

    expect(container.querySelector(".lt-range")).toHaveTextContent(
      "1-25 of 1,204",
    );
    const size = screen.getByRole("combobox", { name: "Rows per page" });
    expect(size).toHaveTextContent(/^25$/);
  });
});
