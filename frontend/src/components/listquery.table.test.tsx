/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProblemError } from "../lib/problem";
import {
  LIST_PAGE_SIZES,
  type ListPage,
  type ListQuery,
  type SavedViewTab,
} from "./listquery";
import {
  emptyPage,
  ListTableHarness,
  type Row,
  render,
} from "./listquery.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/**
 * One saved-view tab as `useSavedViewTabs` builds it: the whole list state the
 * view restores. The defaults are the ones a view saved off an untouched list
 * carries, so a test names only the part it is about.
 */
function savedTab(
  tab: Readonly<{
    id: string;
    label: string;
    q?: string;
    sort?: string;
    filters?: Readonly<Record<string, string>>;
    includeArchived?: boolean;
    perPage?: number;
  }>,
): SavedViewTab {
  return {
    id: tab.id,
    label: tab.label,
    q: tab.q ?? "",
    sort: tab.sort ?? "",
    filters: tab.filters ?? {},
    includeArchived: tab.includeArchived ?? false,
    perPage: tab.perPage ?? LIST_PAGE_SIZES[0],
  };
}

/**
 * A view whose stored blob claims no page size. `POST /views` takes `query` as
 * an open JSON object, so any client can write one, and this is the shape
 * `listStateOf` hands the rail for it.
 */
function tabWithoutPageSize(
  tab: Readonly<{ id: string; label: string }>,
): SavedViewTab {
  return {
    id: tab.id,
    label: tab.label,
    q: "",
    sort: "",
    filters: {},
    includeArchived: false,
  };
}

/**
 * The rail as `/views` hands it over: ordered by name. So a view created or
 * renamed is inserted ahead of the one the reader is looking at.
 */
function InsertableRail({
  fetchPage,
}: Readonly<{
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
}>) {
  const zulu = savedTab({ id: "v-zulu", label: "Zulu", sort: "full_name" });
  const [tabs, setTabs] = useState<readonly SavedViewTab[]>([zulu]);
  return (
    <>
      <button
        type="button"
        onClick={() =>
          setTabs([
            savedTab({ id: "v-alpha", label: "Alpha", sort: "full_name" }),
            zulu,
          ])
        }
      >
        rename
      </button>
      <ListTableHarness
        fetchPage={fetchPage}
        views={[{ label: "list.viewAll" }]}
        dataViews={tabs}
      />
    </>
  );
}

describe("ListTable: query vocabulary", () => {
  it("debounces search input before sending it to fetchPage", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    const search = await screen.findByPlaceholderText("Search");

    vi.useFakeTimers();
    try {
      fireEvent.change(search, { target: { value: "acme" } });

      expect(fetchPage.mock.calls.some(([query]) => query.q === "acme")).toBe(
        false,
      );

      await act(async () => {
        vi.advanceTimersByTime(250);
        await Promise.resolve();
      });

      expect(fetchPage.mock.calls.some(([query]) => query.q === "acme")).toBe(
        true,
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not revert a concurrent archived toggle when the debounced search commits", async () => {
    // The debounce commits the query as it stands when the timer fires, so an
    // archived toggle made while the search is still settling survives it.
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    const search = await screen.findByPlaceholderText("Search");

    vi.useFakeTimers();
    try {
      fireEvent.change(search, { target: { value: "acme" } });

      await act(async () => {
        vi.advanceTimersByTime(100);
      });
      const archived = screen.getByLabelText("Show archived");
      fireEvent.click(archived);

      await act(async () => {
        vi.advanceTimersByTime(250);
        await Promise.resolve();
      });

      const lastCall = fetchPage.mock.calls.at(-1);
      expect(lastCall?.[0].q).toBe("acme");
      expect(lastCall?.[0].includeArchived).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("clicking a sortable column header requests that field from the server", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);

    const sortButton = await screen.findByRole("button", {
      name: "Sort by contacts.name",
    });
    await userEvent.click(sortButton);

    expect(
      fetchPage.mock.calls.some(([query]) => query.sort === "full_name"),
    ).toBe(true);
  });

  it("toggling Show archived requests archived rows", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);

    const archived = await screen.findByLabelText("Show archived");
    await userEvent.click(archived);

    expect(
      fetchPage.mock.calls.some(([query]) => query.includeArchived === true),
    ).toBe(true);
  });

  it("picking a filter chip narrows the query, and clearing it drops the key", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    const { container } = render(
      <ListTableHarness
        fetchPage={fetchPage}
        chips={[
          {
            key: "status",
            label: "lead.filterStatus",
            allLabel: "lead.filterStatusAll",
            options: [
              { value: "new", label: "lead.statusNew" },
              { value: "contacted", label: "lead.statusContacted" },
            ],
          },
        ]}
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Filter" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Status" }));
    await userEvent.click(screen.getByRole("radio", { name: "New" }));

    expect(
      fetchPage.mock.calls.some(([query]) => query.filters.status === "new"),
    ).toBe(true);

    // The applied filter now reads as a row (attribute/condition/value). Its
    // value segment reopens the same value list, showing the chosen label.
    // The query is scoped to the trigger itself, since its own (closed) menu
    // carries a same-labelled option.
    const valueTrigger = container.querySelector<HTMLElement>(".lt-frow-value");
    if (!valueTrigger) {
      throw new Error("the applied filter's value trigger did not render");
    }
    await userEvent.click(valueTrigger);
    await userEvent.click(screen.getByRole("radio", { name: "All statuses" }));

    const lastCall = fetchPage.mock.calls.at(-1);
    expect(lastCall?.[0].filters).not.toHaveProperty("status");
  });
});

describe("ListTable: pending, error and empty states", () => {
  it("keeps the header, toolbar and primary action on screen while the first page loads, and shows placeholder rows in the body", () => {
    const fetchPage = vi.fn(() => new Promise<ListPage<Row>>(() => {}));
    const { container } = render(
      <ListTableHarness
        fetchPage={fetchPage}
        action={<button type="button">New contact</button>}
      />,
    );
    expect(screen.getByRole("button", { name: "New contact" })).toBeTruthy();
    expect(container.querySelectorAll(".lt-loading").length).toBeGreaterThan(0);
    expect(container.querySelector(".lt-bone")).toBeTruthy();
  });

  it("shows the server's error detail and retries on demand", async () => {
    const fetchPage = vi
      .fn<(query: ListQuery, cursor: string | null) => Promise<ListPage<Row>>>()
      // A server problem, not a bare Error: the body reports the detail the
      // API sent. A failure with no problem behind it falls back to the
      // generic copy rather than putting an internal message on the screen.
      .mockRejectedValueOnce(
        new ProblemError({ detail: "missing scope contacts:read" }),
      )
      .mockResolvedValue(emptyPage());
    render(<ListTableHarness fetchPage={fetchPage} />);

    await screen.findByText("Could not load this view. Reload the page.");
    expect(screen.getByText("missing scope contacts:read")).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "Retry" }));

    await screen.findByRole("cell", { name: "No Contacts yet." });
  });

  it("renders the table's own empty state once the list loads with no rows", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);

    await screen.findByRole("cell", { name: "No Contacts yet." });
  });
});

describe("removing an applied filter", () => {
  it("drops the key from the query when the row's Delete filter is used", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        chips={[
          {
            key: "status",
            label: "lead.filterStatus",
            allLabel: "lead.filterStatusAll",
            options: [{ value: "contacted", label: "lead.statusContacted" }],
          },
        ]}
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Filter" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Status" }));
    await userEvent.click(screen.getByRole("radio", { name: "Contacted" }));
    await waitFor(() =>
      expect(fetchPage.mock.calls.some(([query]) => query.filters.status)).toBe(
        true,
      ),
    );

    await userEvent.click(
      screen.getByRole("button", {
        name: "More actions for the Status filter",
      }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Delete filter" }),
    );

    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters).not.toHaveProperty(
        "status",
      ),
    );
  });
});

describe("view tabs — two views can ask for the same thing", () => {
  it("highlights the tab the reader pressed, not the first one that matches", async () => {
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
        views={[
          { label: "list.viewAll" },
          { label: "list.viewMine", sort: "full_name" },
        ]}
        dataViews={[
          savedTab({ id: "v-1", label: "My A-Z", sort: "full_name" }),
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    // The saved view narrows the same way the built-in preset does. Derived
    // from the query alone, the highlight lands on the first match, and the
    // reader's own view never lights up when they pick it.
    await user.click(screen.getByRole("button", { name: "My A-Z" }));
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "My A-Z" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    expect(
      screen.getByRole("button", { name: "Mine" }).getAttribute("aria-pressed"),
    ).toBe("false");
  });
});

describe("saved view tabs restore the whole list state", () => {
  it("restores the archived toggle and the page size the view was saved with", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        views={[{ label: "list.viewAll" }]}
        dataViews={[
          savedTab({
            id: "v-1",
            label: "Closed too",
            includeArchived: true,
            perPage: 50,
          }),
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: "Closed too" }));

    // The toggle and the page size are part of the ListQuery the fetchers read.
    // So restoring them is a new read. A tab that dropped them showed the
    // reader a different list than the one they named.
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].includeArchived).toBe(true),
    );
    expect(fetchPage.mock.calls.at(-1)?.[0].perPage).toBe(50);
    // And the tab stays lit: the archived toggle counts as part of what the
    // view claims. A highlight that ignored it went dark the instant the view
    // it had just restored was applied.
    expect(
      screen
        .getByRole("button", { name: "Closed too" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("false");
  });

  it("keeps the reader's page size for a view that claims none", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        views={[{ label: "list.viewAll" }]}
        dataViews={[
          savedTab({ id: "v-1", label: "Hundred", perPage: 100 }),
          tabWithoutPageSize({ id: "v-2", label: "No claim" }),
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: "Hundred" }));
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].perPage).toBe(100),
    );

    await user.click(screen.getByRole("button", { name: "No claim" }));

    // 100 rows, not the default: a view whose stored blob carries no page size
    // is making no claim about one. Substituting the default there dropped the
    // reader from a hundred rows to twenty-five with no dial moving.
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "No claim" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    expect(fetchPage.mock.calls.at(-1)?.[0].perPage).toBe(100);
  });
});

describe("the rail re-orders under the reader", () => {
  it("keeps the same view selected when another is inserted ahead of it", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    render(<InsertableRail fetchPage={fetchPage} />);
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(screen.getByRole("button", { name: "Zulu" }));
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "Zulu" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );

    // Both views narrow identically, so the position is the only thing that
    // tells them apart. Tracked by index, the highlight follows the slot and
    // lands on whichever view moved into it.
    await user.click(screen.getByRole("button", { name: "rename" }));

    expect(
      screen.getByRole("button", { name: "Zulu" }).getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      screen
        .getByRole("button", { name: "Alpha" })
        .getAttribute("aria-pressed"),
    ).toBe("false");
  });

  it("renders both of two views that share a name", async () => {
    // Two saved views may carry the same name, so a rail keyed on the label
    // hands React two children with one key. React warns about that, because
    // what it does with them from then on is not defined.
    const warnings: string[] = [];
    const console_error = vi
      .spyOn(console, "error")
      .mockImplementation((...args: readonly unknown[]) => {
        warnings.push(args.map(String).join(" "));
      });
    try {
      const fetchPage = vi.fn(
        async (_query: ListQuery, _cursor: string | null) => emptyPage(),
      );
      render(
        <ListTableHarness
          fetchPage={fetchPage}
          views={[{ label: "list.viewAll" }]}
          dataViews={[
            savedTab({ id: "v-1", label: "Customers", sort: "full_name" }),
            savedTab({ id: "v-2", label: "Customers", sort: "-created_at" }),
          ]}
        />,
      );
      await waitFor(() => expect(fetchPage).toHaveBeenCalled());

      expect(screen.getAllByRole("button", { name: "Customers" })).toHaveLength(
        2,
      );
      expect(warnings.filter((line) => line.includes("same key"))).toEqual([]);
    } finally {
      console_error.mockRestore();
    }
  });
});

describe("paging a filtered list", () => {
  it("keeps going forward instead of snapping back to page 1", async () => {
    const user = userEvent.setup();
    const page = (from: number) => ({
      data: Array.from({ length: 25 }, (_, i) => ({
        id: `r-${from + i}`,
        name: `Row ${from + i}`,
      })),
      page: { next_cursor: `c-${from + 25}`, has_more: true },
    });
    let served = 0;
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      page(25 * served++),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        chips={[
          {
            key: "owner",
            label: "list.owner",
            allLabel: "list.filterOwnerAll",
            options: [{ value: "owner_id:u-1", label: "list.filterOwnerMe" }],
          },
        ]}
      />,
    );
    await waitFor(() => expect(screen.getByText("Row 0")).toBeTruthy());

    // Next twice, each press waiting for its page to render rather than for a
    // timer to fire. A zero-duration sleep races React Query's commit. A test
    // that sometimes clicks Next on a page that has not arrived sometimes
    // passes for the wrong reason.
    //
    // The table resets to page 1 whenever `chosen` changes identity. So a
    // chosen object rebuilt on every render reset on every render, and the
    // list flipped between the first two pages forever. One press hides that:
    // the reset lands on a page whose content happens to match. It takes two.
    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(screen.getByText("Row 50")).toBeTruthy());
    expect(screen.queryByText("Row 0")).toBeNull();
  });
});
