/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { precedes } from "../testing/domorder";
import { type ListColumn, ListTable } from "./listtable";

// The toolbar's Display menu: the dials that decide how the grid is DRAWN —
// row density and which optional columns stand. listtable.test.tsx proves the
// query dials beside it; this file is the drawing half alone.

afterEach(cleanup);

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

type Row = { id: string; name: string; value: number };

const rows: readonly Row[] = [{ id: "r1", name: "Row 01", value: 1 }];

const columns: readonly ListColumn<Row>[] = [
  { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
  { key: "value", header: "Value", cell: (row) => String(row.value) },
];

/** The identity column alone: a table with nothing optional to hide. */
const identityOnly: readonly ListColumn<Row>[] = [columns[0]];

function table() {
  return screen.getByRole("table");
}

function menu() {
  return within(screen.getByRole("group", { name: "Display" }));
}

describe("the Display menu", () => {
  it("reports on its trigger whether it is standing", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );

    const trigger = screen.getByRole("button", { name: "Display" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(menu().getByText("Density")).toBeTruthy();

    await user.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
  });

  it("names both groups, and drops the column group where nothing is optional", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Display" }));
    expect(menu().getByText("Density")).toBeTruthy();
    expect(menu().getByText("Shown columns")).toBeTruthy();
    // And a rung UNDER the head that names the menu: three labels at one weight
    // read as three menus in a box rather than one question in two parts.
    const panel = screen.getByRole("group", { name: "Display" });
    expect(panel.querySelectorAll(".lt-mhead")).toHaveLength(1);
    expect(panel.querySelectorAll(".lt-mgroup.t-caption")).toHaveLength(2);

    cleanup();
    render(
      <ListTable
        rows={rows}
        columns={identityOnly}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Display" }));
    // Density is every grid's dial; a heading over an empty set is a group the
    // reader looks into for options that were never there.
    expect(menu().getByText("Density")).toBeTruthy();
    expect(menu().queryByText("Shown columns")).toBeNull();
  });

  it("tightens the rows from its Compact tick", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Display" }));
    const compact = menu().getByRole("checkbox", { name: "Compact" });
    expect(compact).not.toBeChecked();
    expect(table()).not.toHaveClass("dense");

    await user.click(compact);
    expect(compact).toBeChecked();
    expect(table()).toHaveClass("dense");

    await user.click(compact);
    expect(table()).not.toHaveClass("dense");
  });

  it("hides and re-shows a column without closing", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const trigger = screen.getByRole("button", { name: "Display" });
    await user.click(trigger);
    // The identity column is not optional, so it is not on offer.
    expect(menu().queryByRole("checkbox", { name: "Name" })).toBeNull();

    await user.click(menu().getByRole("checkbox", { name: "Value" }));
    expect(screen.queryByRole("columnheader", { name: "Value" })).toBeNull();

    // A set is built in one visit, not one visit per tick.
    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    await user.click(menu().getByRole("checkbox", { name: "Value" }));
    expect(screen.getByRole("columnheader", { name: "Value" })).toBeTruthy();
  });

  it("stands before the screen's own tools", () => {
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        tools={
          <button type="button" key="save">
            Save view
          </button>
        }
      />,
    );
    const display = screen.getByRole("button", { name: "Display" });
    const save = screen.getByRole("button", { name: "Save view" });
    expect(precedes(display, save)).toBe(true);
  });

  it("is withheld from a body that draws no grid", () => {
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        body={<div>Board</div>}
        bodyOwnsPaging
      />,
    );
    // Both dials describe a grid, and this body has none to describe.
    expect(screen.queryByRole("button", { name: "Display" })).toBeNull();
  });
});

// ONE MENU OPEN AT A TIME IS A PROPERTY OF THE ROW, so every dial standing on
// it reads the same state. The Display menu used to keep an `open` of its own,
// and the two answers did not agree: Sort and Display stood open side by side.
describe("the toolbar's menus, against each other", () => {
  const sortable: readonly ListColumn<Row>[] = [
    { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
    {
      key: "value",
      header: "Value",
      cell: (row) => String(row.value),
      sort: "value",
    },
  ];

  const chips = [
    {
      key: "owner",
      label: "Owner",
      allLabel: "Any owner",
      options: [{ value: "u1", label: "Ada" }],
    },
  ];

  function row() {
    return rtlRender(
      <LocaleProvider initial="en">
        <ListTable
          rows={rows}
          columns={sortable}
          rowKey={(one: Row) => one.id}
          unit="rows"
          sort={{ value: "", onChange: () => undefined }}
          chips={chips}
          chosen={{}}
          onChipChange={() => undefined}
        />
      </LocaleProvider>,
    );
  }

  const dial = (name: string | RegExp) => screen.getByRole("button", { name });

  it("closes an open Sort when Display is opened, and the other way about", async () => {
    const user = userEvent.setup();
    row();

    await user.click(dial(/^Sort(:|$)/));
    expect(dial(/^Sort(:|$)/)).toHaveAttribute("aria-expanded", "true");

    await user.click(dial("Display"));
    expect(dial("Display")).toHaveAttribute("aria-expanded", "true");
    expect(dial(/^Sort(:|$)/)).toHaveAttribute("aria-expanded", "false");

    await user.click(dial(/^Sort(:|$)/));
    expect(dial(/^Sort(:|$)/)).toHaveAttribute("aria-expanded", "true");
    expect(dial("Display")).toHaveAttribute("aria-expanded", "false");
  });

  it("closes an open Display when Filter is opened", async () => {
    const user = userEvent.setup();
    row();

    await user.click(dial("Display"));
    await user.click(dial("Filter"));

    expect(dial("Filter")).toHaveAttribute("aria-expanded", "true");
    expect(dial("Display")).toHaveAttribute("aria-expanded", "false");
  });

  // The surface owns the state, so its one Escape path reaches this menu too —
  // it used to need a handler of its own beside it.
  it("closes the Display menu on Escape", async () => {
    const user = userEvent.setup();
    row();

    await user.click(dial("Display"));
    await user.keyboard("{Escape}");

    expect(dial("Display")).toHaveAttribute("aria-expanded", "false");
  });
});
