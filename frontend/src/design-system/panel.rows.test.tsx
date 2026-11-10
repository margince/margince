/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createPortal } from "react-dom";
import { afterEach, describe, expect, it } from "vitest";
import { PanelGroupHead, PanelRow } from "./panel";

afterEach(cleanup);

describe("PanelGroupHead", () => {
  it("puts its id on the heading, so a table in the group can be named by it", () => {
    render(<PanelGroupHead title="Notes" level="h3" id="group-notes" />);
    expect(screen.getByRole("heading", { name: "Notes", level: 3 }).id).toBe(
      "group-notes",
    );
  });
});

// The rule and the hover are two shapes. A row that lights up under the
// pointer tells the reader it is pressable, which a ruled block meant to be
// read is not. So the default is inert, and a caller opts in only when the
// whole row is one press target.
describe("PanelRow separates the hairline from the press", () => {
  it("draws an inert row by default", () => {
    const { container } = render(<PanelRow>Renewal date</PanelRow>);
    const row = container.querySelector(".panel-row");
    expect(row).not.toBeNull();
    expect(row?.classList.contains("panel-row-interactive")).toBe(false);
  });

  it("marks the row a press target when the caller says it is one", () => {
    const { container } = render(
      <PanelRow interactive>
        <button type="button">Q4 — renewals</button>
      </PanelRow>,
    );
    expect(
      container
        .querySelector(".panel-row")
        ?.classList.contains("panel-row-interactive"),
    ).toBe(true);
  });

  // The caller's own class survives the variant: a screen names its row for
  // layout, and the two spellings have to coexist on one element.
  it("keeps the caller's class beside the variant", () => {
    const { container } = render(
      <PanelRow interactive className="panel-row-on">
        Q3
      </PanelRow>,
    );
    const row = container.querySelector(".panel-row");
    expect(row?.classList.contains("panel-row-interactive")).toBe(true);
    expect(row?.classList.contains("panel-row-on")).toBe(true);
  });

  it("marks a row of a record list apart from a press target", () => {
    const { container } = render(<PanelRow record>Deep tier</PanelRow>);
    const row = container.querySelector(".panel-row");
    expect(row?.classList.contains("panel-row-record")).toBe(true);
    expect(row?.classList.contains("panel-row-interactive")).toBe(false);
  });

  it("leaves a plain row out of the record list", () => {
    const { container } = render(<PanelRow>Renewal date</PanelRow>);
    expect(
      container
        .querySelector(".panel-row")
        ?.classList.contains("panel-row-record"),
    ).toBe(false);
  });
});

type OpeningRowProps = { opened: string[]; pressed: string[] };

describe("PanelRow onOpen opens on a press that lands on no control", () => {
  function OpeningRow({ opened, pressed }: Readonly<OpeningRowProps>) {
    return (
      <PanelRow record onOpen={() => opened.push("Pricing playbook")}>
        <span>Pricing playbook</span>
        <button type="button" onClick={() => pressed.push("menu")}>
          Actions for Pricing playbook
        </button>
        {createPortal(
          <div role="menu">
            <span>Archive set</span>
          </div>,
          document.body,
        )}
      </PanelRow>
    );
  }

  it("opens the row on a press on its text, and marks it with the pointer", async () => {
    const user = userEvent.setup();
    const opened: string[] = [];
    const { container } = render(<OpeningRow opened={opened} pressed={[]} />);
    await user.click(screen.getByText("Pricing playbook"));
    expect(opened).toEqual(["Pricing playbook"]);
    expect(
      container
        .querySelector(".panel-row")
        ?.classList.contains("panel-row-opens"),
    ).toBe(true);
  });

  it("leaves a press on a button inside the row to that button", async () => {
    const user = userEvent.setup();
    const opened: string[] = [];
    const pressed: string[] = [];
    render(<OpeningRow opened={opened} pressed={pressed} />);
    await user.click(
      screen.getByRole("button", { name: "Actions for Pricing playbook" }),
    );
    expect(pressed).toEqual(["menu"]);
    expect(opened).toEqual([]);
  });

  // React bubbles a portal's click through the row, though the menu is not in its DOM.
  it("ignores a press inside a menu the row portalled out", async () => {
    const user = userEvent.setup();
    const opened: string[] = [];
    render(<OpeningRow opened={opened} pressed={[]} />);
    await user.click(within(screen.getByRole("menu")).getByText("Archive set"));
    expect(opened).toEqual([]);
  });

  it("draws no pointer on a row without onOpen", () => {
    const { container } = render(<PanelRow record>Deep tier</PanelRow>);
    expect(
      container
        .querySelector(".panel-row")
        ?.classList.contains("panel-row-opens"),
    ).toBe(false);
  });
});
