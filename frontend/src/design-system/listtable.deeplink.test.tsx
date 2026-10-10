/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { ListTable } from "./listtable";

afterEach(cleanup);

const COLUMNS = [
  { key: "id", header: "Name", cell: (row: { id: string }) => row.id },
];
const READ = 50;
const ALL = 409;

const rowsUpTo = (count: number) =>
  Array.from({ length: count }, (_, at) => ({ id: `r-${at}` }));

// The cursor read the list screens make: a fixed step per read, no jumping.
function Walk({
  page,
  reads,
}: Readonly<{ page: number; reads: { count: number } }>) {
  const [loaded, setLoaded] = useState(READ);
  return (
    <LocaleProvider initial="en">
      <ListTable
        rows={rowsUpTo(loaded)}
        columns={COLUMNS}
        rowKey={(row) => row.id}
        unit="rows"
        page={page}
        onPage={() => undefined}
        hasMore={loaded < ALL}
        onLoadMore={() => {
          reads.count += 1;
          setLoaded((have) => Math.min(ALL, have + READ));
        }}
      />
    </LocaleProvider>
  );
}

describe("a link to a page past the rows in hand", () => {
  it("loads on until the page the address names is the one drawn", async () => {
    const reads = { count: 0 };
    render(<Walk page={12} reads={reads} />);

    await waitFor(() => expect(screen.getByText("r-275")).toBeTruthy());
    expect(screen.queryByText("r-175")).toBeNull();
  });

  it("stops asking once the page is in hand", async () => {
    const reads = { count: 0 };
    render(<Walk page={2} reads={reads} />);

    await waitFor(() => expect(screen.getByText("r-25")).toBeTruthy());
    expect(reads.count).toBe(0);
  });

  it("lands on the last page when the address names one that does not exist", async () => {
    const reads = { count: 0 };
    render(<Walk page={99} reads={reads} />);

    await waitFor(() => expect(screen.getByText("r-408")).toBeTruthy());
    expect(reads.count).toBe(Math.ceil((ALL - READ) / READ));
  });
});
