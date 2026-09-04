// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { PageAsideProvider, PageAsideToggle, usePageAside } from "./pageaside";

const KEY = "margince.pageAside.collapsed";

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

// A record screen's shape: it claims the pane, draws its content only while
// the pane is open, and carries the switch at the end of its tab row.
function Record({ available = true }: Readonly<{ available?: boolean }>) {
  const details = usePageAside(available);
  return (
    <>
      <PageAsideToggle />
      {details.open && <aside>Nine fields and the tags.</aside>}
    </>
  );
}

function record(available?: boolean) {
  const view = render(
    <LocaleProvider initial="en">
      <PageAsideProvider>
        <Record available={available} />
      </PageAsideProvider>
    </LocaleProvider>,
  );
  return {
    ...view,
    pane: () => view.container.querySelector("aside"),
  };
}

// The details pane is where a reader goes for the attributes, not what they
// open a record to see, so it starts folded until they say otherwise — and
// what they say is remembered.
describe("the details pane is closed until asked", () => {
  it("starts folded when nothing is remembered", () => {
    const { pane } = record();
    expect(pane()).toBeNull();
  });

  it("starts open when the reader last left it open", () => {
    localStorage.setItem(KEY, "0");
    const { pane } = record();
    expect(pane()).not.toBeNull();
  });

  it("remembers a fold and an unfold", async () => {
    const user = userEvent.setup();
    const { pane, getByRole } = record();
    await user.click(getByRole("button", { name: "Details" }));
    expect(pane()).not.toBeNull();
    expect(localStorage.getItem(KEY)).toBe("0");
    await user.click(getByRole("button", { name: "Details" }));
    expect(pane()).toBeNull();
    expect(localStorage.getItem(KEY)).toBe("1");
  });

  it("starts folded when storage refuses to answer", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage refused");
    });
    const { pane } = record();
    expect(pane()).toBeNull();
  });
});

// A switch for a pane that does not exist is a control that does nothing: a
// screen whose composer holds the pane's place offers neither.
describe("the switch goes with the pane", () => {
  it("is absent while the screen has no pane to offer", () => {
    localStorage.setItem(KEY, "0");
    const { pane, queryByRole } = record(false);
    expect(pane()).toBeNull();
    expect(queryByRole("button")).toBeNull();
  });

  it("starts open on a surface with no reader to remember", () => {
    const view = render(
      <LocaleProvider initial="en">
        <PageAsideProvider open>
          <Record />
        </PageAsideProvider>
      </LocaleProvider>,
    );
    expect(view.container.querySelector("aside")).not.toBeNull();
  });
});
