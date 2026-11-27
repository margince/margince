/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

const pageButton = (number: number) =>
  screen.queryByRole("button", { name: `Page ${number}` });

describe("the pager's page numbers", () => {
  it("run to the last page the server's total names, past the rows in hand", () => {
    render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
        total={4363}
      />,
    );
    expect(pageButton(3)).toBeTruthy();
    expect(pageButton(175)).toBeTruthy();
    expect(pageButton(176)).toBeNull();
  });

  // A gap after the pages in hand says more follow, so they do not read as the list.
  it("ends in a gap while the server sends no total and more rows exist", () => {
    const { container } = render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
      />,
    );
    expect(pageButton(1)).toBeTruthy();
    expect(pageButton(2)).toBeTruthy();
    expect(pageButton(3)).toBeNull();
    const gaps = [...container.querySelectorAll(".lt-gap")].map(
      (slot) => slot.textContent,
    );
    expect(gaps).toContain("…");
    expect(screen.getByRole("button", { name: "Next" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("name the last page of a list whose every row is in hand", () => {
    render(
      <ListTable
        rows={testRows(150)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(pageButton(6)).toBeTruthy();
  });

  it("follow the rows in hand when they outrun a total read earlier", () => {
    render(
      <ListTable
        rows={testRows(100)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
        total={60}
      />,
    );
    expect(pageButton(4)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Next" })).toHaveProperty(
      "disabled",
      false,
    );
  });
});

describe("the count line", () => {
  it("says the server's total, not the rows it has loaded", () => {
    const { container } = render(
      <ListTable
        rows={testRows(200)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
        total={8372}
      />,
    );
    expect(container.querySelector(".lt-count")).toHaveTextContent(
      /^1-25 of 8,372$/,
    );
    // "Loaded" is the caveat for a number the client counted itself. An exact
    // total needs none, and carrying it would make the real figure read as a floor.
    expect(screen.queryByText(/loaded/)).toBeNull();
  });

  // An endpoint that does not count is not an endpoint reporting zero. Every
  // list that never sends a total keeps the "loaded" line.
  it("falls back to the loaded count when the server sends no total", () => {
    const { container } = render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
      />,
    );
    expect(container.querySelector(".lt-count")).toHaveTextContent(
      /^1-25 of 50 loaded$/,
    );
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
    expect(screen.queryByText(/loaded/)).toBeNull();
  });

  it("says an empty list's zero once, in its body, and keeps the order above it", () => {
    render(
      <ListTable
        rows={[]}
        columns={[{ ...columns[0], sort: "name" }]}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "name", onChange: () => {} }}
      />,
    );
    expect(screen.getAllByText(/No rows yet/)).toHaveLength(1);
    expect(screen.getByRole("table").textContent).toContain("No rows yet.");
    expect(screen.getByText("Sorted by Name")).toBeTruthy();
  });
});

describe("the range under the pager", () => {
  const range = (container: HTMLElement) =>
    container.querySelector(".lt-range")?.textContent;

  it("says the rows of the page on screen out of the server's total", async () => {
    const { container } = render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
        total={1024}
      />,
    );
    expect(range(container)).toBe("1-25 of 1,024");
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Page 2" }));
    expect(range(container)).toBe("26-50 of 1,024");
  });

  it("says the rows in hand are only the loaded ones while the server sends no total", () => {
    const { container } = render(
      <ListTable
        rows={testRows(50)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore
      />,
    );
    expect(range(container)).toBe("1-25 of 50 loaded");
  });

  it("groups the figures for the reader's language", () => {
    const { container } = rtlRender(
      <LocaleProvider initial="de">
        <ListTable
          rows={testRows(50)}
          columns={columns}
          rowKey={(row) => row.id}
          unit="rows"
          hasMore
          total={11388}
        />
      </LocaleProvider>,
    );
    expect(range(container)).toBe("1-25 von 11.388");
  });
});
