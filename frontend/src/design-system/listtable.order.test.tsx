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
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import { openView, openViewStep } from "../testing/listview.testkit";
import { type ListColumn, ListTable } from "./listtable";
import { orderedColumns } from "./listtable.order";

// The Columns step: a grip moves a column and a tick hides one, the grid
// follows, and both hold between visits under the table's name.

afterEach(() => {
  cleanup();
  localStorage.clear();
});

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

type Row = { id: string; name: string; city: string; stage: string };

const rows: readonly Row[] = [
  { id: "r1", name: "Ada", city: "Berlin", stage: "Lead" },
];

const columns: readonly ListColumn<Row>[] = [
  { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
  { key: "city", header: "City", cell: (row) => row.city },
  { key: "stage", header: "Stage", cell: (row) => row.stage },
];

function grid(widthsKey?: string) {
  return (
    <ListTable
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="rows"
      widthsKey={widthsKey}
    />
  );
}

function headers(): string[] {
  return within(screen.getByRole("table"))
    .getAllByRole("columnheader")
    .map((cell) => cell.textContent ?? "");
}

describe("column order", () => {
  it("moves a column by keyboard on its grip, and the grid follows", async () => {
    const user = userEvent.setup();
    render(grid());
    await openViewStep(user, "Columns");

    const grip = screen.getByRole("button", { name: "Move Stage, 2 of 2" });
    grip.focus();
    await user.keyboard("{ArrowUp}");

    expect(headers()).toEqual(["Name", "Stage", "City"]);
    expect(
      screen.getByRole("button", { name: "Move Stage, 1 of 2" }),
    ).toBeTruthy();
  });

  it("keeps the order after a remount under the same table name", async () => {
    const user = userEvent.setup();
    const first = render(grid("order-test"));
    await openViewStep(user, "Columns");
    screen.getByRole("button", { name: "Move Stage, 2 of 2" }).focus();
    await user.keyboard("{ArrowUp}");
    first.unmount();

    render(grid("order-test"));
    expect(headers()).toEqual(["Name", "Stage", "City"]);
  });

  it("keeps a column the reader hid hidden after a remount under the same table name", async () => {
    const user = userEvent.setup();
    const first = render(grid("hidden-test"));
    const step = within(await openViewStep(user, "Columns"));
    await user.click(step.getByRole("checkbox", { name: "City" }));
    first.unmount();

    render(grid("hidden-test"));
    expect(headers()).toEqual(["Name", "Stage"]);
  });

  it("shows every column again on a table with no name", async () => {
    const user = userEvent.setup();
    const first = render(grid());
    const step = within(await openViewStep(user, "Columns"));
    await user.click(step.getByRole("checkbox", { name: "City" }));
    first.unmount();

    render(grid());
    expect(headers()).toEqual(["Name", "City", "Stage"]);
  });

  it("ignores a stored key whose column is gone, and never moves the fixed column", () => {
    writeStored(
      { family: STORAGE_KEYS.tableOrder, member: "stale" },
      JSON.stringify(["gone", "stage", "name", "city"]),
    );
    render(grid("stale"));
    expect(headers()).toEqual(["Name", "Stage", "City"]);
  });
});

describe("row density", () => {
  const dense = (member: string) => ({
    family: STORAGE_KEYS.tableDense,
    member,
  });

  it("reads a table's stored Compact on mount", () => {
    writeStored(dense("dense-test"), "true");
    render(grid("dense-test"));
    expect(screen.getByRole("table")).toHaveClass("dense");
  });

  it("stores Compact under the table's name and leaves another table loose", async () => {
    const user = userEvent.setup();
    const first = render(grid("dense-leads"));
    const panel = within(await openView(user));
    await user.click(panel.getByRole("button", { name: "Compact" }));
    expect(readStored(dense("dense-leads"))).toBe("true");
    first.unmount();

    const again = render(grid("dense-leads"));
    expect(screen.getByRole("table")).toHaveClass("dense");
    again.unmount();

    render(grid("dense-contacts"));
    expect(screen.getByRole("table")).not.toHaveClass("dense");
  });

  it("draws loose rows when the stored value is not a boolean", () => {
    writeStored(dense("dense-garbage"), '"yes"');
    render(grid("dense-garbage"));
    expect(screen.getByRole("table")).not.toHaveClass("dense");
  });

  it("keeps Compact for this mount only on a table with no name", async () => {
    const user = userEvent.setup();
    const first = render(grid());
    const panel = within(await openView(user));
    await user.click(panel.getByRole("button", { name: "Compact" }));
    expect(screen.getByRole("table")).toHaveClass("dense");
    first.unmount();

    render(grid());
    expect(screen.getByRole("table")).not.toHaveClass("dense");
  });
});

describe("orderedColumns", () => {
  it("puts new columns after the stored ones, in declared order", () => {
    const keys = orderedColumns(columns, ["stage"]).map((column) => column.key);
    expect(keys).toEqual(["name", "stage", "city"]);
  });
});
