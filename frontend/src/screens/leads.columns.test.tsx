/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import type { ListColumn } from "../design-system/listtable";
import { LocaleProvider, useT } from "../i18n";
import { leadColumns } from "./leads.columns";

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Anna Example",
  company_name: "Northwind Traders",
  tags: [{ tag_id: "t-1", name: "Product A", color: "teal" }],
  status: "new",
  score: 0,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  legal_hold: false,
  writable: true,
  open_task_count: 0,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

/** Renders one column's cell for `lead`, and hands the column back. */
function Cell({
  columnKey,
  onColumn,
}: Readonly<{ columnKey: string; onColumn: (c: ListColumn<Lead>) => void }>) {
  const t = useT();
  const zone = useRecordZone();
  const column = leadColumns(t, "en", zone, undefined).find(
    (c) => c.key === columnKey,
  );
  if (!column) throw new Error(`no ${columnKey} column`);
  onColumn(column);
  return <div data-testid="cell">{column.cell(lead)}</div>;
}

function renderCell(columnKey: string): ListColumn<Lead> {
  let found: ListColumn<Lead> | undefined;
  render(
    <LocaleProvider initial="en">
      <Cell columnKey={columnKey} onColumn={(c) => (found = c)} />
    </LocaleProvider>,
  );
  if (!found) throw new Error("column not rendered");
  return found;
}

afterEach(cleanup);

describe("the lead table's columns", () => {
  it("give the company a column of its own, sorted by the company the server holds", () => {
    const column = renderCell("company");
    expect(column.sort).toBe("company_name");
    expect(screen.getByTestId("cell").textContent).toBe("Northwind Traders");
  });

  it("no longer trail the company after the name as a caption", () => {
    renderCell("name");
    expect(screen.getByTestId("cell").textContent).toBe("Anna Example");
  });

  it("show the tags a lead is filed under", () => {
    renderCell("tags");
    expect(screen.getByTestId("cell").textContent).toContain("Product A");
  });
});
