// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
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
describe("the details pane is open until folded", () => {
  it("starts open when nothing is remembered", () => {
    const { pane } = record();
    expect(pane()).not.toBeNull();
  });

  it("starts folded when the reader last folded it", () => {
    localStorage.setItem(KEY, "1");
    const { pane } = record();
    expect(pane()).toBeNull();
  });

  it("remembers a fold and an unfold, and says which it offers", async () => {
    const user = userEvent.setup();
    const { pane, getByRole } = record();
    await user.click(getByRole("button", { name: "Hide details" }));
    expect(pane()).toBeNull();
    expect(localStorage.getItem(KEY)).toBe("1");
    await user.click(getByRole("button", { name: "Show details" }));
    expect(pane()).not.toBeNull();
    expect(localStorage.getItem(KEY)).toBe("0");
  });

  it("starts open when storage refuses to answer", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage refused");
    });
    const { pane } = record();
    expect(pane()).not.toBeNull();
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
