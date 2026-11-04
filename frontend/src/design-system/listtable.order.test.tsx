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
import { STORAGE_KEYS, writeStored } from "../lib/storage";
import { type ListColumn, ListTable } from "./listtable";
import { orderedColumns } from "./listtable.order";

// Column order in the Display menu. The reader moves a column by its grip and
// the grid follows. The order holds between visits under the table's name.

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
    await user.click(screen.getByRole("button", { name: "Display" }));

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
    await user.click(screen.getByRole("button", { name: "Display" }));
    screen.getByRole("button", { name: "Move Stage, 2 of 2" }).focus();
    await user.keyboard("{ArrowUp}");
    first.unmount();

    render(grid("order-test"));
    expect(headers()).toEqual(["Name", "Stage", "City"]);
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

describe("orderedColumns", () => {
  it("puts new columns after the stored ones, in declared order", () => {
    const keys = orderedColumns(columns, ["stage"]).map((column) => column.key);
    expect(keys).toEqual(["name", "stage", "city"]);
  });
});
