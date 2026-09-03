// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { PageAside, PageAsideProvider, PageAsideRegion } from "./pageaside";

const KEY = "margince.pageAside.collapsed";

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

function column() {
  const view = render(
    <LocaleProvider initial="en">
      <PageAsideProvider>
        <PageAsideRegion />
        <PageAside>
          <p>Nine fields and the tags.</p>
        </PageAside>
      </PageAsideProvider>
    </LocaleProvider>,
  );
  const aside = view.container.querySelector("aside.pageaside");
  if (!aside) {
    throw new Error("the column did not render");
  }
  return { aside, ...view };
}

// The details column is where a reader goes for the attributes, not what they
// open a record to see, so it starts folded until they say otherwise — and
// what they say is remembered.
describe("the details column is closed until asked", () => {
  it("starts folded when nothing is remembered", () => {
    const { aside } = column();
    expect(aside.classList.contains("collapsed")).toBe(true);
  });

  it("starts open when the reader last left it open", () => {
    localStorage.setItem(KEY, "0");
    const { aside } = column();
    expect(aside.classList.contains("collapsed")).toBe(false);
  });

  it("remembers a fold and an unfold", async () => {
    const user = userEvent.setup();
    const { aside, getByRole } = column();
    await user.click(getByRole("button", { name: /Show/ }));
    expect(aside.classList.contains("collapsed")).toBe(false);
    expect(localStorage.getItem(KEY)).toBe("0");
    await user.click(getByRole("button", { name: /Hide/ }));
    expect(aside.classList.contains("collapsed")).toBe(true);
    expect(localStorage.getItem(KEY)).toBe("1");
  });

  it("starts folded when storage refuses to answer", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage refused");
    });
    const { aside } = column();
    expect(aside.classList.contains("collapsed")).toBe(true);
  });
});
