/** @vitest-environment happy-dom */
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

// `held` keeps a read in flight until the test lets it land, as a network does.
type Probe = { reads: number; pages: number[]; held?: (() => void)[] };

// The cursor read the list screens make: a fixed step per read, no jumping.
function Walk({
  page,
  probe,
  bodyOwnsPaging = false,
}: Readonly<{ page?: number; probe: Probe; bodyOwnsPaging?: boolean }>) {
  const [loaded, setLoaded] = useState(READ);
  return (
    <LocaleProvider initial="en">
      <ListTable
        rows={rowsUpTo(loaded)}
        columns={COLUMNS}
        rowKey={(row) => row.id}
        unit="rows"
        page={page}
        onPage={(next) => probe.pages.push(next)}
        hasMore={loaded < ALL}
        bodyOwnsPaging={bodyOwnsPaging}
        onLoadMore={() => {
          probe.reads += 1;
          const land = () => setLoaded((have) => Math.min(ALL, have + READ));
          if (probe.held) {
            probe.held.push(land);
          } else {
            land();
          }
        }}
      />
    </LocaleProvider>
  );
}

const probeOf = (): Probe => ({ reads: 0, pages: [] });

describe("a link to a page past the rows in hand", () => {
  it("loads on until the page the address names is the one drawn", async () => {
    render(<Walk page={12} probe={probeOf()} />);

    await waitFor(() => expect(screen.getByText("r-275")).toBeTruthy());
    expect(screen.queryByText("r-175")).toBeNull();
  });

  it("stops asking once the page is in hand", async () => {
    const probe = probeOf();
    render(<Walk page={2} probe={probe} />);

    await waitFor(() => expect(screen.getByText("r-25")).toBeTruthy());
    expect(probe.reads).toBe(0);
  });

  it("brings the address back to the last page when it names one that does not exist", async () => {
    const probe = probeOf();
    render(<Walk page={99999} probe={probe} />);

    await waitFor(() => expect(screen.getByText("r-408")).toBeTruthy());
    expect(probe.reads).toBe(Math.ceil((ALL - READ) / READ));
    await waitFor(() => expect(probe.pages).toEqual([17]));
  });

  it("leaves a body that owns its paging to walk its own pages", () => {
    const probe = probeOf();
    render(<Walk page={12} probe={probe} bodyOwnsPaging />);

    expect(probe.reads).toBe(0);
    expect(probe.pages).toEqual([]);
  });
});

describe("pressing Next on the last loaded page", () => {
  it("asks for the next read once while it is in flight", async () => {
    const user = userEvent.setup();
    const probe: Probe = { reads: 0, pages: [], held: [] };
    render(<Walk probe={probe} />);

    // The first press moves to page 2, which is in hand; the second steps past it.
    await user.click(screen.getByRole("button", { name: "Next" }));
    await user.click(screen.getByRole("button", { name: "Next" }));
    expect(probe.reads).toBe(1);

    act(() => {
      for (const land of probe.held?.splice(0) ?? []) {
        land();
      }
    });
    await waitFor(() => expect(screen.getByText("r-50")).toBeTruthy());
    expect(probe.reads).toBe(1);
  });
});
