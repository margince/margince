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
import { openView, openViewStep } from "../testing/listview.testkit";
import { type ListColumn, ListTable } from "./listtable";

// The Columns and Density rows: how the grid is drawn, not what is in it.

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

function grid(cols: readonly ListColumn<Row>[] = columns) {
  return render(
    <ListTable
      rows={rows}
      columns={cols}
      rowKey={(row) => row.id}
      unit="rows"
    />,
  );
}

describe("the Columns row and the Compact toggle", () => {
  it("say what they hold, and Columns stands only where a column is optional", async () => {
    const user = userEvent.setup();
    grid();
    const panel = within(await openView(user));
    expect(panel.getByRole("button", { name: "Columns, 1 of 1" })).toBeTruthy();
    expect(panel.getByRole("button", { name: "Compact" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );

    cleanup();
    grid(identityOnly);
    const bare = within(await openView(user));
    expect(bare.queryByRole("button", { name: /^Columns/ })).toBeNull();
    expect(bare.getByRole("button", { name: "Compact" })).toBeTruthy();
  });

  // Which rows exist reads before how they are drawn.
  it("tightens the rows from the Compact toggle, after Show archived", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        archived={{ checked: false, onChange: () => undefined }}
      />,
    );
    const panel = within(await openView(user));
    const compact = panel.getByRole("button", { name: "Compact" });
    expect(
      precedes(panel.getByRole("button", { name: "Show archived" }), compact),
    ).toBe(true);
    expect(compact).not.toHaveTextContent("On");
    expect(compact.querySelector(".lt-vdot")).toBeNull();
    expect(table()).not.toHaveClass("dense");

    await user.click(compact);
    expect(compact).toHaveAttribute("aria-pressed", "true");
    expect(compact).toHaveTextContent("On");
    expect(table()).toHaveClass("dense");

    await user.click(compact);
    expect(table()).not.toHaveClass("dense");
  });

  it("hides and re-shows a column without leaving the step", async () => {
    const user = userEvent.setup();
    grid();
    const step = within(await openViewStep(user, "Columns"));
    // The identity column is not optional, so it is not on offer.
    expect(step.queryByRole("checkbox", { name: "Name" })).toBeNull();

    await user.click(step.getByRole("checkbox", { name: "Value" }));
    expect(screen.queryByRole("columnheader", { name: "Value" })).toBeNull();

    // A set is built in one visit, not one visit per tick.
    await user.click(step.getByRole("checkbox", { name: "Value" }));
    expect(screen.getByRole("columnheader", { name: "Value" })).toBeTruthy();
  });

  it("is withheld from a body that draws no grid", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={rows}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        body={<div>Board</div>}
        bodyOwnsPaging
        archived={{ checked: false, onChange: () => undefined }}
      />,
    );
    // Both dials describe a grid, and this body has none to describe.
    const panel = within(await openView(user));
    expect(panel.queryByRole("button", { name: /^Columns/ })).toBeNull();
    expect(panel.queryByRole("button", { name: "Compact" })).toBeNull();
  });
});

describe("a display dial off its default", () => {
  const more: readonly ListColumn<Row>[] = [
    ...columns,
    { key: "note", header: "Note", cell: () => "", initiallyHidden: true },
  ];

  it("marks Columns once a column is shown, hidden or moved, and only then", async () => {
    const user = userEvent.setup();
    grid(more);
    const panel = within(await openView(user));
    expect(
      panel
        .getByRole("button", { name: "Columns, 1 of 2" })
        .querySelector(".lt-vdot"),
    ).toBeNull();

    let step = within(await openViewStep(user, "Columns"));
    await user.click(step.getByRole("checkbox", { name: "Note" }));
    await user.click(step.getByRole("button", { name: /^Columns/ }));
    expect(
      panel.getByRole("button", { name: "Columns, 2 of 2, changed" }),
    ).toBeTruthy();

    step = within(await openViewStep(user, "Columns"));
    await user.click(step.getByRole("checkbox", { name: "Note" }));
    await user.click(step.getByRole("button", { name: /^Columns/ }));
    expect(panel.getByRole("button", { name: "Columns, 1 of 2" })).toBeTruthy();

    step = within(await openViewStep(user, "Columns"));
    step.getByRole("button", { name: "Move Value, 1 of 2" }).focus();
    await user.keyboard("{ArrowDown}");
    await user.click(step.getByRole("button", { name: /^Columns/ }));
    expect(
      panel.getByRole("button", { name: "Columns, 1 of 2, changed" }),
    ).toBeTruthy();
  });
});
