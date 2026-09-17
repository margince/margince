/** @vitest-environment happy-dom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListColumn, ListTable } from "./listtable";

// The count line's one sentence: how many rows this list holds. It has two
// sources and they say different things — a server total describes the whole
// matching set, while the rows in hand describe only what has been fetched —
// so the tests below are about which one the line speaks and how it says so.
//
// The defect they pin: /contacts fetches 200 rows per read, so a workspace of
// 8,372 contacts rendered "1-25 of 200", which reads as the workspace holding
// 200 contacts.

afterEach(cleanup);

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

type Row = { id: string; name: string };

function testRows(count: number): Row[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `r${index + 1}`,
    name: `Row ${String(index + 1).padStart(2, "0")}`,
  }));
}

const columns: readonly ListColumn<Row>[] = [
  { key: "name", header: "Name", cell: (row) => row.name, fixed: true },
];

describe("the count line", () => {
  it("says the server's total, not the rows it has loaded", () => {
    render(
      <ListTable
        rows={testRows(200)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
        total={8372}
      />,
    );
    expect(screen.getByText(/1–25 of 8,372 rows/)).toBeTruthy();
    // "Loaded so far" is the caveat for a number the client counted itself. An
    // exact total needs none, and carrying it would make the real figure read
    // as a floor.
    expect(screen.queryByText(/loaded so far/)).toBeNull();
  });

  // An endpoint that does not count is not an endpoint reporting zero. Every
  // list that never sends a total keeps the honest loaded-so-far line.
  it("falls back to the loaded count when the server sends no total", () => {
    render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
      />,
    );
    expect(screen.getByText(/1–25 of 50 rows loaded so far/)).toBeTruthy();
  });

  // Zero is a real answer and must not be read as "did not count": a filter
  // matching nothing says so. Both paths print 0 here and only one is honest,
  // which is why the total is carried as an optional number rather than a
  // count that defaults to zero.
  it("reports a server total of zero as an empty list", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        total={0}
      />,
    );
    expect(screen.getAllByText(/no rows/i).length).toBeGreaterThan(0);
    expect(screen.queryByText(/loaded so far/)).toBeNull();
  });
});
