/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import type { ListColumn } from "../../design-system/listtable";
import { LocaleProvider, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
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
  row = lead,
}: Readonly<{
  columnKey: string;
  onColumn: (c: ListColumn<Lead>) => void;
  row?: Lead;
}>) {
  const t = useT();
  const zone = useRecordZone();
  const column = leadColumns(t, "en", zone, undefined).find(
    (c) => c.key === columnKey,
  );
  if (!column) throw new Error(`no ${columnKey} column`);
  onColumn(column);
  return <div data-testid="cell">{column.cell(row)}</div>;
}

// The cell sits in a clickable row, as the table draws it. A test can then see
// whether a click inside the cell also opened the row.
function renderCell(
  columnKey: string,
  row?: Lead,
  onRow: () => void = () => undefined,
): ListColumn<Lead> {
  let found: ListColumn<Lead> | undefined;
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* biome-ignore lint/a11y/useKeyWithClickEvents: a stand-in for the table row's own click */}
        {/* biome-ignore lint/a11y/noStaticElementInteractions: a stand-in for the table row's own click */}
        <div onClick={onRow}>
          <Cell columnKey={columnKey} onColumn={(c) => (found = c)} row={row} />
        </div>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  if (!found) throw new Error("column not rendered");
  return found;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the lead table's columns", () => {
  it("count open tasks in the singular for one and the plural for the rest", () => {
    renderCell("nextTask", { ...lead, open_task_count: 1 });
    expect(screen.getByTestId("cell").textContent).toContain("1 open task");
    expect(screen.getByTestId("cell").textContent).not.toContain("tasks");
    cleanup();
    renderCell("nextTask", { ...lead, open_task_count: 3 });
    expect(screen.getByTestId("cell").textContent).toContain("3 open tasks");
  });

  it("give the company a column of its own, sorted by the company the server holds", () => {
    const column = renderCell("company");
    expect(column.sort).toBe("company_name");
    expect(screen.getByTestId("cell").textContent).toBe("Northwind Traders");
  });

  it("open a linked company under its own name, without opening the row", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              data: [{ id: "c-7", display_name: "Northwind Holding" }],
              next_cursor: null,
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          ),
      ),
    );
    const onRow = vi.fn();
    renderCell("company", { ...lead, company_id: "c-7" }, onRow);
    const link = await screen.findByRole("link", { name: "Northwind Holding" });
    expect(link.getAttribute("href")).toContain("c-7");
    expect(document.querySelector('[title="Northwind Traders"]')).toBeNull();
    fireEvent.click(link);
    expect(onRow).not.toHaveBeenCalled();
  });

  it("show an unlinked company as plain text, with its name on hover", () => {
    renderCell("company");
    expect(screen.queryByRole("link")).toBeNull();
    expect(screen.getByTestId("cell").textContent).toBe("Northwind Traders");
    expect(
      document.querySelector('[title="Northwind Traders"]'),
    ).not.toBeNull();
  });

  it("no longer trail the company after the name as a caption", () => {
    renderCell("name");
    expect(screen.getByTestId("cell").textContent).toBe("Anna Example");
  });

  it("show the tags a lead is filed under", () => {
    renderCell("tags");
    expect(screen.getByTestId("cell").textContent).toContain("Product A");
  });

  it("print the origin's label in the origin column and sort by it", () => {
    const column = renderCell("origin", {
      ...lead,
      origin: "referral",
      origin_label: "Referral",
    });
    expect(column.sort).toBe("origin");
    expect(screen.getByTestId("cell").textContent).toBe("Referral");
  });

  it("say an unclassified lead has no origin, whatever wrote the row", () => {
    renderCell("origin", {
      ...lead,
      origin: null,
      source: "mirror:csv:pipedrive-export:lead:l-7",
    });
    expect(screen.getByTestId("cell").textContent).toBe("Not set");
  });
});
