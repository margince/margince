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
  archivable = true,
  showingArchived = false,
  pending = false,
  problem,
}: Readonly<{
  rows: Row[];
  search?: string;
  archivable?: boolean;
  showingArchived?: boolean;
  pending?: boolean;
  problem?: string;
}>) {
  return (
    <ListTable
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="rows"
      hideShortChrome
      pending={pending}
      problem={problem}
      search={{ value: search, answered: search, onChange: () => undefined }}
      archived={
        archivable
          ? { checked: showingArchived, onChange: () => undefined }
          : undefined
      }
    />
  );
}

describe("a short list that hides its chrome", () => {
  it("draws its rows without a count, dials or pager", () => {
    render(<Short rows={testRows(3)} />);
    expect(screen.getByText("Row 3")).toBeTruthy();
    expect(screen.queryByText(/1 to 3 of 3 rows/)).toBeNull();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull();
  });

  // The rows in hand never include archived ones, so a list of only archived
  // records would read as empty with no way in.
  it("keeps the archived switch while it hides the other dials", () => {
    render(<Short rows={[]} />);
    expect(
      screen.getByRole("checkbox", { name: "Show archived" }),
    ).toBeTruthy();
    expect(screen.queryByRole("searchbox")).toBeNull();
  });

  it("draws no switch for a list that offers no archiving", () => {
    render(<Short rows={testRows(3)} archivable={false} />);
    expect(screen.queryByRole("checkbox")).toBeNull();
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

  it("keeps every dial while it shows archived rows too", () => {
    render(<Short rows={testRows(3)} showingArchived />);
    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(screen.getByText(/1 to 3 of 3 rows/)).toBeTruthy();
  });

  // Not yet known to be short: the rows that arrive may run past one page.
  it("draws no empty line while its rows load", () => {
    render(<Short rows={[]} pending />);
    expect(screen.queryByText("No rows yet.")).toBeNull();
    expect(screen.getByRole("searchbox")).toBeTruthy();
  });

  it("states a failed read instead of an empty line", () => {
    render(<Short rows={[]} problem="The rows could not be read." />);
    expect(screen.getByText("The rows could not be read.")).toBeTruthy();
    expect(screen.queryByText("No rows yet.")).toBeNull();
  });

  it("keeps every dial past the smallest page size", () => {
    render(<Short rows={testRows(40)} />);
    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(screen.getByText(/1 to 25 of 40 rows/)).toBeTruthy();
  });
});
