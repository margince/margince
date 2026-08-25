/** @vitest-environment jsdom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { ListTable } from "./listtable";

afterEach(cleanup);

const COLUMNS = [
  { key: "id", header: "Name", cell: (row: { id: string }) => row.id },
];
const ROWS = Array.from({ length: 60 }, (_, at) => ({ id: `r-${at}` }));

function Grid({
  page,
  onPage,
  search,
}: Readonly<{
  page?: number;
  onPage?: (next: number) => void;
  search: string;
}>) {
  return (
    <LocaleProvider initial="en">
      <ListTable
        rows={ROWS}
        columns={COLUMNS}
        rowKey={(row) => row.id}
        unit="rows"
        page={page}
        onPage={onPage}
        search={{ value: search, onChange: () => undefined }}
      />
    </LocaleProvider>
  );
}

function body(container: HTMLElement): HTMLElement {
  const rows = container.querySelector<HTMLElement>(".lt-scroll");
  if (!rows) {
    throw new Error("the table drew no scrolling body");
  }
  return rows;
}

/**
 * Narrowing a list changes what page one means, so the table goes back to it.
 * What it must NOT do from there is touch where the reader was in the rows.
 *
 * This effect also runs on the way OUT. Opening a record gives the list one last
 * render against an address that is no longer its own, every dial it reads then
 * being empty — which is indistinguishable, from in here, from a reader clearing
 * the search box. The page survives that either way: it lives in the address and
 * is derived again on the way back. An offset written then is MEMORY overwritten,
 * and the reader comes back to the top of a list they had scrolled a long way
 * down. Whether that last render happens at all is a matter of timing, which is
 * why the defect came back as "sometimes".
 */
describe("the reset a narrowing triggers", () => {
  it("takes the reader back to page one", () => {
    const moves: number[] = [];
    const { rerender } = render(
      <Grid page={4} onPage={(next) => moves.push(next)} search="" />,
    );
    expect(moves).toEqual([]);

    rerender(<Grid page={4} onPage={(next) => moves.push(next)} search="ac" />);
    expect(moves).toEqual([1]);
  });

  it("leaves the reader's place in the rows alone", () => {
    const { container, rerender } = render(<Grid search="" />);
    body(container).scrollTop = 3400;

    rerender(<Grid search="ac" />);
    expect(body(container).scrollTop).toBe(3400);
  });

  it("does not fire for arriving, however many times the effect runs", () => {
    // An effect runs on arrival, twice on arrival under StrictMode, and again
    // for any dial that settles a tick after mount. A reset that counted runs
    // read all three as a reader turning something.
    const moves: number[] = [];
    const { rerender } = render(
      <Grid page={6} onPage={(next) => moves.push(next)} search="acme" />,
    );
    rerender(
      <Grid page={6} onPage={(next) => moves.push(next)} search="acme" />,
    );
    rerender(
      <Grid page={6} onPage={(next) => moves.push(next)} search="acme" />,
    );
    expect(moves).toEqual([]);
  });
});
