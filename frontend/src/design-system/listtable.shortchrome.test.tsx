/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListColumn, ListTable } from "./listtable";

afterEach(cleanup);

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

type Row = { id: string; name: string };

function testRows(count: number): Row[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `r${index + 1}`,
    name: `Row ${index + 1}`,
  }));
}

const columns: readonly ListColumn<Row>[] = [
  { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
];

function Short({
  rows,
  search = "",
}: Readonly<{ rows: Row[]; search?: string }>) {
  return (
    <ListTable
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="rows"
      hideShortChrome
      search={{ value: search, onChange: () => undefined }}
      archived={{ checked: false, onChange: () => undefined }}
    />
  );
}

describe("a short list that hides its chrome", () => {
  it("draws its rows without a count, dials or pager", () => {
    render(<Short rows={testRows(3)} />);
    expect(screen.getByText("Row 3")).toBeTruthy();
    expect(screen.queryByText(/1 to 3 of 3 rows/)).toBeNull();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.queryByRole("checkbox", { name: /archived/i })).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull();
  });

  it("says it is empty in one line, with no table of headings", () => {
    render(<Short rows={[]} />);
    expect(screen.getByText("No rows yet.")).toBeTruthy();
    expect(screen.queryByRole("columnheader")).toBeNull();
  });

  // Hiding the search box a reader typed into would strand them in the result.
  it("keeps every dial once something narrows it", () => {
    render(<Short rows={[]} search="acme" />);
    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(screen.getByText(/No rows match these filters/)).toBeTruthy();
  });

  it("keeps every dial past the smallest page size", () => {
    render(<Short rows={testRows(40)} />);
    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(screen.getByText(/1 to 25 of 40 rows/)).toBeTruthy();
  });
});
