/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import {
  LIST_PAGE_SIZES,
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "./listquery";
import {
  emptyPage,
  ListTableHarness,
  type Row,
  render,
} from "./listquery.testkit";

// The shared list foundation (P-14): keyset pagination via useListQuery, and
// ListTable binding that query to the design-system list surface. The
// debounce is real (setTimeout) so we drive it with fake timers, never a
// real sleep (craft T11).

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function Harness({
  fetchPage,
}: Readonly<{
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
}>) {
  const { rows, hasMore, loadMore } = useListQuery<Row>({
    key: "harness",
    fetchPage,
  });
  return (
    <div>
      <ul>
        {rows.map((row) => (
          <li key={row.id}>{row.id}</li>
        ))}
      </ul>
      <span data-testid="has-more">{String(hasMore)}</span>
      <button type="button" onClick={loadMore}>
        more
      </button>
    </div>
  );
}

/**
 * Two lists on one route, each owning its own half of the address.
 *
 * The settings Data-model tab is the real case: the products table and the
 * offer-template table are drawn together, and a flat parameter space described
 * both at once.
 */
function TwoListHarness({
  left,
  right,
}: Readonly<{
  left: (query: ListQuery, cursor: string | null) => Promise<ListPage<Row>>;
  right: (query: ListQuery, cursor: string | null) => Promise<ListPage<Row>>;
}>) {
  const one = useListQuery<Row>({
    key: "left",
    fetchPage: left,
    initialSort: "name",
    paramScope: "left",
  });
  const other = useListQuery<Row>({
    key: "right",
    fetchPage: right,
    initialSort: "locale",
    paramScope: "right",
  });
  const columns = [
    {
      key: "name",
      header: "contacts.name",
      cell: (row: Row) => row.name,
      sort: "name",
    },
  ];
  return (
    <>
      <ListTable
        state={one}
        unit="unit.products"
        columns={columns}
        rowKey={(row) => row.id}
      />
      <ListTable
        state={other}
        unit="unit.offerTemplates"
        columns={columns}
        rowKey={(row) => row.id}
      />
    </>
  );
}

describe("useListQuery", () => {
  it("accumulates rows across pages and tracks has_more", async () => {
    const fetchPage = vi.fn(
      async (_query: ListQuery, cursor: string | null) => {
        if (cursor === null) {
          return {
            data: [{ id: "a", name: "Anna" }],
            page: { next_cursor: "c1", has_more: true },
          };
        }
        return {
          data: [{ id: "b", name: "Bob" }],
          page: { next_cursor: null, has_more: false },
        };
      },
    );
    render(<Harness fetchPage={fetchPage} />);

    await screen.findByText("a");
    expect(screen.getByTestId("has-more").textContent).toBe("true");
    expect(screen.queryByText("b")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "more" }));

    await screen.findByText("b");
    expect(screen.getByText("a")).toBeTruthy();
    expect(screen.getByTestId("has-more").textContent).toBe("false");
  });
});

describe("the search box follows the address", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("shows the q an address arrives with, not the one the reader left", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    const search = await screen.findByPlaceholderText("Search");

    vi.useFakeTimers();
    try {
      fireEvent.change(search, { target: { value: "stripe" } });
      await act(async () => {
        vi.advanceTimersByTime(250);
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }
    await waitFor(() => expect(search).toHaveProperty("value", "stripe"));

    // The address moves under a screen that stays mounted: Back, Forward, or a
    // link to the same list narrowed differently. The box has to follow it, or
    // it shows words the rows are not answering.
    await act(async () => {
      window.location.hash = "#/contacts?q=brandt";
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    await waitFor(() => expect(search).toHaveProperty("value", "brandt"));
    expect(fetchPage.mock.calls.at(-1)?.[0].q).toBe("brandt");
  });

  it("drops a word still settling when the reader leaves the list it was typed into", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    const search = await screen.findByPlaceholderText("Search");

    vi.useFakeTimers();
    try {
      // Typed, and NOT yet committed.
      fireEvent.change(search, { target: { value: "acme" } });

      // The reader leaves for an address that carries no `q` either — Back to a
      // differently sorted view of the same list. `q` never moved, so following
      // `q` alone would leave the timer with its appointment kept.
      await act(async () => {
        window.location.hash = "#/contacts?sort=full_name";
        window.dispatchEvent(new HashChangeEvent("hashchange"));
      });
      await act(async () => {
        vi.advanceTimersByTime(250);
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() => expect(search).toHaveProperty("value", ""));
    // The word lands on no list at all, least of all the one the reader is on.
    expect(fetchPage.mock.calls.every(([query]) => query.q !== "acme")).toBe(
      true,
    );
  });
});

describe("two lists on one route", () => {
  afterEach(() => {
    window.location.hash = "";
  });

  it("keep separate dials, so neither narrows the other's read", async () => {
    const left = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    const right = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<TwoListHarness left={left} right={right} />);

    // Each list seeds its OWN opening sort, under its own prefix, and neither
    // seeding wipes the other's.
    await waitFor(() => {
      expect(window.location.hash).toContain("left.sort=name");
      expect(window.location.hash).toContain("right.sort=locale");
    });
    expect(left.mock.calls.at(-1)?.[0].sort).toBe("name");
    expect(right.mock.calls.at(-1)?.[0].sort).toBe("locale");

    const searches = await screen.findAllByPlaceholderText("Search");
    vi.useFakeTimers();
    try {
      fireEvent.change(searches[0], { target: { value: "acme" } });
      await act(async () => {
        vi.advanceTimersByTime(250);
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() => {
      expect(left.mock.calls.some(([query]) => query.q === "acme")).toBe(true);
    });
    // The other list never asked for it. Sharing one `q` is what made the
    // product search narrow the template table too.
    expect(right.mock.calls.every(([query]) => query.q === "")).toBe(true);
    expect(window.location.hash).toContain("left.q=acme");
    expect(window.location.hash).not.toContain("right.q=");
  });

  it("a sort one list cannot answer never reaches the other", async () => {
    const left = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    const right = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    window.location.hash = "#/settings?left.sort=sku&right.sort=locale";
    render(<TwoListHarness left={left} right={right} />);

    await waitFor(() => {
      expect(left.mock.calls.at(-1)?.[0].sort).toBe("sku");
    });
    // `sku` is not in the template list's sort vocabulary, and reaching it
    // would be a 422 the reader never asked for.
    expect(right.mock.calls.every(([query]) => query.sort !== "sku")).toBe(
      true,
    );
  });
});

describe("listFetchLimit — one read carries several rendered pages", () => {
  it("fetches whole rendered pages, never a remainder, for every offered size", () => {
    for (const perPage of LIST_PAGE_SIZES) {
      const limit = listFetchLimit(perPage);
      // A remainder would leave a last page shorter than the size the footer
      // names; over the ceiling the server clamps and the pager offers numbers
      // with no rows behind them.
      expect(limit % perPage).toBe(0);
      expect(limit).toBeLessThanOrEqual(200);
      expect(limit).toBeGreaterThan(200 - perPage);
    }
  });

  it("fills the default page size's strip in one read", () => {
    expect(listFetchLimit(25) / 25).toBe(8);
  });
});

describe("useListQuery — the page size is part of the server query", () => {
  it("hands every fetcher the page size the reader picked", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(
      async (_query: ListQuery, _cursor: string | null) => ({
        data: [] as Row[],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());
    expect(fetchPage.mock.calls[0]?.[0].perPage).toBe(25);

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Rows per page" }),
      "50 per page",
    );

    // Every screen reads its `limit` off this one value. A fetcher that kept a
    // literal instead would render a page size the server never returned.
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].perPage).toBe(50),
    );
  });
});

describe("the owner dial — one question the server answers three ways", () => {
  it("swaps the owner parameter instead of stacking two of them", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(
      async (_query: ListQuery, _cursor: string | null) => ({
        data: [] as Row[],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        chips={[
          {
            key: "owner",
            label: "list.owner",
            allLabel: "list.filterOwnerAll",
            options: [
              { value: "owner_id:u-1", label: "list.filterOwnerMe" },
              { value: "unassigned:true", label: "list.filterOwnerUnassigned" },
            ],
          },
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Owner" }));
    await user.click(screen.getByRole("radio", { name: "Owned by you" }));

    // The option carries the parameter it sets, so the chip writes `owner_id`
    // rather than a filter named after the chip itself.
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters.owner_id).toBe("u-1"),
    );
    // And the chip reads back as chosen: a dial that narrows the list and then
    // renders as "Any owner" looks like a filter that did not take.
    expect(
      screen.getByRole("group", { name: "Owner: Owned by you" }),
    ).toBeTruthy();
  });
});
