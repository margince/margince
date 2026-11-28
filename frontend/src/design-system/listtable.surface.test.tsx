/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
import { openViewStep, viewButton } from "../testing/listview.testkit";
import { type ListChip, ListTable, type ListView } from "./listtable";
import { PAGER_SLOTS, pagerSlots } from "./listtable.paging";
import { columns, render, testRows } from "./listtable.testkit";
import { pickOption } from "./select-testing";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A header whose verbs fold into one menu must behave like the row of verbs it
// replaced. By default the menu defers its children to the first open. That is
// right per row and wrong here. A verb that reads its opening state once, at
// mount, is dead until somebody presses the menu. `#/deals/new` is such an
// address: its whole meaning is "the create form is open". Below this width it
// opened nothing, and pressing the menu then opened a form nobody asked for.
function MountReporter({ onMount }: Readonly<{ onMount: () => void }>) {
  // A verb that does its work at MOUNT, which is the shape `CreateAction`'s
  // `listCreate.open` has: read once in `useState`, so a late mount reads it late.
  useEffect(() => {
    onMount();
  }, [onMount]);
  return <button type="button">New thing</button>;
}

describe("views", () => {
  it("switching a view reports its index and applies its sort and filters", async () => {
    const user = userEvent.setup();
    const chips: readonly ListChip[] = [
      {
        key: "status",
        label: "Status",
        allLabel: "All statuses",
        options: [{ value: "new", label: "New" }],
      },
    ];
    const views: readonly ListView[] = [
      { label: "All" },
      { label: "New leads", sort: "name", filters: { status: "new" } },
    ];
    const onViewChange = vi.fn();
    const onChipChange = vi.fn();
    const sortOnChange = vi.fn();

    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        onChipChange={onChipChange}
        sort={{ value: "", onChange: sortOnChange }}
        views={views}
        activeView={0}
        onViewChange={onViewChange}
      />,
    );

    await user.click(screen.getByRole("button", { name: "New leads" }));
    expect(onViewChange).toHaveBeenCalledWith(1);
    expect(sortOnChange).toHaveBeenCalledWith("name");
    expect(onChipChange).toHaveBeenCalledWith("status", "new");
  });
});

// A list screen is a table and nothing else. So the page's name belongs in the
// header that carries the tabs and the count, not on its own line above the
// card. The tests hold the count of headings: one h1 when a title is given, and
// none when it is not. The shell prints the heading for every screen that does
// not pass one. A surface that always drew its own would name those pages
// twice.
describe("the page's name in the header", () => {
  it("names the page in the header's own level-1 heading", () => {
    const { container } = render(
      <ListTable
        title="Contacts"
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const heading = screen.getByRole("heading", { level: 1, name: "Contacts" });
    expect(container.querySelector(".lt-head")?.contains(heading)).toBe(true);
  });

  it("draws no heading for a surface that is not the page", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
  });
});

// The reason for a refused verb sits under the head, so the head keeps one row
// and the verb names the reason as its description.
describe("a refused head action", () => {
  it("says why under the head, and the refused verb is described by it", () => {
    const { container } = render(
      <ListTable
        title="Companies"
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        refusal={{ id: "why-refused", text: "Your seat is read-only." }}
        action={
          <button type="button" disabled aria-describedby="why-refused">
            New company
          </button>
        }
      />,
    );
    const why = screen.getByText("Your seat is read-only.");
    expect(container.querySelector(".lt-head")?.contains(why)).toBe(false);
    expect(
      screen.getByRole("button", { name: "New company" }),
    ).toHaveAccessibleDescription("Your seat is read-only.");
  });
});

describe("a folded header's verbs are live", () => {
  function stubNarrowViewport() {
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: /max-width:\s*1100px/.test(query),
      media: query,
      addEventListener: () => {},
      removeEventListener: () => {},
    }));
  }

  it("mounts the action without the menu being opened", async () => {
    stubNarrowViewport();
    const mounted = vi.fn();
    render(
      <ListTable
        title="Deals"
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        action={<MountReporter onMount={mounted} />}
      />,
    );
    // The fold happened: the verbs are behind one menu rather than in the row.
    expect(
      await screen.findByRole("button", {
        name: englishCopy["list.headActions"],
      }),
    ).toBeTruthy();
    // And the verb is mounted anyway. The check is on the mount, not on the
    // button being visible, because the fold sets `hidden` on it. Visibility is
    // the wrong question and would pass on a dead control.
    expect(mounted).toHaveBeenCalledTimes(1);
  });
});

describe("pagination", () => {
  it("shows only the first 25 of 60 rows, with a 3-button pager", () => {
    render(
      <ListTable
        rows={testRows(60)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const dataRows = screen.getAllByRole("row").slice(1);
    expect(dataRows).toHaveLength(25);
    expect(
      screen
        .getByRole("button", { name: "Page 1" })
        .getAttribute("aria-current"),
    ).toBe("page");
    expect(screen.getByRole("button", { name: "Page 3" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Page 4" })).toBeNull();
  });

  it("clicking page 2 renders rows 26 through 50", async () => {
    const user = userEvent.setup();
    const data = testRows(60);
    render(
      <ListTable
        rows={data}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    await user.click(screen.getByRole("button", { name: "Page 2" }));
    expect(screen.getByText(data[25].name)).toBeTruthy();
    expect(screen.queryByText(data[0].name)).toBeNull();
  });

  // The pager is navigation, so a reader who moves by region should find it as
  // one. A page button named by its bare digit names nothing outside the row's
  // context, and a screen reader does not have that context.
  it("names itself as navigation, and each page as a page", () => {
    render(
      <ListTable
        rows={testRows(60)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        perPage={25}
      />,
    );
    const pager = screen.getByRole("navigation", { name: "Pages" });
    expect(within(pager).getByRole("button", { name: "Page 2" })).toBeTruthy();
    // The digit is still what a sighted reader sees; only the name is fuller.
    expect(
      within(pager).getByRole("button", { name: "Page 2" }).textContent,
    ).toBe("2");
  });

  it("reports a new rows-per-page to the caller instead of re-slicing rows itself", async () => {
    const data = testRows(60);
    const onPerPage = vi.fn();
    render(
      <ListTable
        rows={data}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        perPage={25}
        onPerPage={onPerPage}
      />,
    );
    const user = userEvent.setup();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: englishCopy["table.rowsPerPage"] }),
      "50 per page",
    );
    // The page size is the server's page size: the table reports the choice and
    // the caller re-asks with it, never reslicing the rows it holds.
    expect(onPerPage).toHaveBeenCalledWith(50);
  });

  it("slices its own rows when the caller offers no page-size handler", async () => {
    render(
      <ListTable
        rows={testRows(60)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const picker = screen.getByRole("combobox", {
      name: englishCopy["table.rowsPerPage"],
    });
    expect(picker).toHaveProperty("disabled", false);
    expect(screen.getAllByRole("row")).toHaveLength(26);

    // Every row is already in hand, so there is no wire to re-ask and slicing
    // them is the whole of what this dial means here.
    await pickOption(userEvent.setup(), picker, "50 per page");
    expect(screen.getAllByRole("row")).toHaveLength(51);
  });

  it("puts the current page between its neighbours and keeps page one reachable", () => {
    // Page one is where a reader who has lost their place goes back to.
    // Walking there one Prev at a time is not going back.
    expect(pagerSlots(5, 49)).toEqual([1, "gap", 4, 5, 6, "gap", 49]);
    expect(pagerSlots(30, 49)).toEqual([1, "gap", 29, 30, 31, "gap", 49]);
  });

  it("widens the window at either end so the strip keeps seven slots", () => {
    expect(pagerSlots(1, 49)).toEqual([1, 2, 3, 4, 5, "gap", 49]);
    expect(pagerSlots(4, 49)).toEqual([1, 2, 3, 4, 5, "gap", 49]);
    expect(pagerSlots(46, 49)).toEqual([1, "gap", 45, 46, 47, 48, 49]);
    expect(pagerSlots(49, 49)).toEqual([1, "gap", 45, 46, 47, 48, 49]);
  });

  it("holds one width so Next stays where the reader clicked it", () => {
    for (const lastPage of [8, 9, 40, 137]) {
      for (let current = 1; current <= lastPage; current += 1) {
        expect(pagerSlots(current, lastPage)).toHaveLength(7);
      }
    }
  });

  it("skips at least two pages behind every gap", () => {
    for (const lastPage of [8, 9, 40]) {
      for (let current = 1; current <= lastPage; current += 1) {
        const slots = pagerSlots(current, lastPage);
        slots.forEach((slot, index) => {
          if (slot === "gap") {
            const before = Number(slots[index - 1]);
            const after = Number(slots[index + 1]);
            expect(after - before).toBeGreaterThan(2);
          }
        });
      }
    }
  });

  it("narrows a short strip to five slots, the window to the current page", () => {
    const phone = (current: number, open = false) =>
      pagerSlots(current, 10, open, PAGER_SLOTS.short);
    expect(phone(1)).toEqual([1, 2, 3, "gap", 10]);
    expect(phone(3)).toEqual([1, 2, 3, "gap", 10]);
    expect(phone(4)).toEqual([1, "gap", 4, "gap", 10]);
    expect(phone(7)).toEqual([1, "gap", 7, "gap", 10]);
    expect(phone(8)).toEqual([1, "gap", 8, 9, 10]);
    expect(phone(10)).toEqual([1, "gap", 8, 9, 10]);
    expect(pagerSlots(2, 5, false, PAGER_SLOTS.short)).toEqual([1, 2, 3, 4, 5]);
    expect(phone(5, true)).toEqual([1, "gap", 5, "gap", 10, "gap"]);
  });

  it("shows every page of a list seven pages long or shorter", () => {
    expect(pagerSlots(4, 7)).toEqual([1, 2, 3, 4, 5, 6, 7]);
    expect(pagerSlots(2, 3)).toEqual([1, 2, 3]);
    expect(pagerSlots(1, 1)).toEqual([1]);
  });

  it("follows the pages in hand with a gap while the end is not known", () => {
    expect(pagerSlots(1, 2, true)).toEqual([1, 2, "gap"]);
    expect(pagerSlots(1, 12, true)).toEqual([1, 2, 3, 4, 5, "gap", 12, "gap"]);
    for (let current = 1; current <= 12; current += 1) {
      expect(pagerSlots(current, 12, true)).toHaveLength(8);
    }
  });

  it("reaches page one from the strip rather than by walking Prev", async () => {
    const user = userEvent.setup();
    const { container } = render(
      <ListTable
        rows={testRows(250)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const slots = () => container.querySelectorAll(".lt-pager > *").length;
    const atFirstPage = slots();

    await user.click(screen.getByRole("button", { name: "Page 5" }));
    await user.click(screen.getByRole("button", { name: "Page 6" }));
    // The strip is drawn, not only counted, at the same length on every page.
    expect(slots()).toBe(atFirstPage);

    await user.click(screen.getByRole("button", { name: "Page 1" }));
    expect(
      screen
        .getByRole("button", { name: "Page 1" })
        .getAttribute("aria-current"),
    ).toBe("page");
  });

  it("stops the window at the ends rather than numbering pages it has no rows for", () => {
    render(
      <ListTable
        rows={testRows(60)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(screen.getByRole("button", { name: "Page 3" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Page 4" })).toBeNull();
  });

  it("disables Next on the last loaded page when hasMore is false, and enables it (calling onLoadMore) when hasMore is true", async () => {
    const user = userEvent.setup();
    const data = testRows(60);
    const onLoadMore = vi.fn();
    const { rerender } = render(
      <ListTable
        rows={data}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        hasMore={false}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Page 3" }));
    expect(
      screen.getByRole("button", { name: "Next" }).hasAttribute("disabled"),
    ).toBe(true);

    rerender(
      <LocaleProvider initial="en">
        <ListTable
          rows={data}
          columns={columns}
          rowKey={(row) => row.id}
          unit="rows"
          hasMore
          onLoadMore={onLoadMore}
        />
      </LocaleProvider>,
    );
    const next = screen.getByRole("button", { name: "Next" });
    expect(next.hasAttribute("disabled")).toBe(false);
    await user.click(next);
    expect(onLoadMore).toHaveBeenCalled();
  });
});

describe("count line", () => {
  it("reads as a range and names the sorted column", () => {
    render(
      <ListTable
        rows={testRows(60)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "name", onChange: () => {} }}
      />,
    );
    expect(screen.getByText("1-25 of 60, sorted by Name")).toBeTruthy();
  });
});

describe("a row as a link", () => {
  it("makes the identity cell a real link, so the row can be opened in a new tab", () => {
    render(
      <ListTable
        rows={testRows(2)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        rowHref={(row) => `#/rows/${row.id}`}
      />,
    );
    const [first] = screen.getAllByRole("link");
    expect(first.getAttribute("href")).toBe(`/#/rows/${testRows(2)[0].id}`);
  });

  it("leaves the cell as plain text when the caller names no address", () => {
    render(
      <ListTable
        rows={testRows(2)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("does not navigate the current page as well when the link is followed", () => {
    const onRowClick = vi.fn();
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        rowHref={(row) => `#/rows/${row.id}`}
        onRowClick={onRowClick}
      />,
    );
    // A modifier-click opens the new tab, and it must not also move this one.
    // The row's handler is what would move it, so the link keeps the click to
    // itself.
    fireEvent.click(screen.getByRole("link"), { metaKey: true });
    expect(onRowClick).not.toHaveBeenCalled();
  });
});

describe("dismissing a popup from the keyboard", () => {
  const chips: readonly ListChip[] = [
    {
      key: "status",
      label: "Status",
      allLabel: "All statuses",
      options: [{ value: "new", label: "New" }],
    },
  ];

  it("closes the View panel on Escape from a step and puts focus back on View", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
      />,
    );
    await openViewStep(user, "Filters");

    await user.keyboard("{Escape}");

    const view = await viewButton();
    expect(view.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(view);
  });

  it("closes an applied filter's menu on Escape and puts focus back on its segment", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        chosen={{ status: "new" }}
      />,
    );
    const segment = screen.getByRole("button", {
      name: "More actions for the Status filter",
    });
    await user.click(segment);
    expect(segment.getAttribute("aria-expanded")).toBe("true");

    await user.keyboard("{Escape}");

    expect(segment.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(segment);
  });
});

describe("empty state", () => {
  it("offers to clear filters only when the list is filtered", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        search={{ value: "acme", answered: "acme", onChange: () => {} }}
      />,
    );
    expect(screen.getByText("No rows match these filters.")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeTruthy();
  });

  it("shows the plain none-yet copy when nothing is filtered", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const table = screen.getByRole("table");
    expect(within(table).getByText("No rows yet.")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeNull();
  });

  it("does not tell the count line nothing exists when a dial is hiding it", () => {
    // Both sentences were on screen at once, and one of them was false. The
    // body said "no companies match these filters" while the line above it
    // said "No companies yet". A search that matches nothing says nothing
    // about whether the workspace has any.
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "name", onChange: () => {} }}
        search={{ value: "acme", answered: "acme", onChange: () => {} }}
      />,
    );
    expect(screen.queryByText(/No rows yet/)).toBeNull();
    expect(screen.getByText("No rows match these filters.")).toBeTruthy();
    // The order is still true, and still worth saying. It now opens the line
    // rather than continuing one. So it starts in upper case, with no comma
    // and nothing on its left.
    expect(screen.getByText("Sorted by Name")).toBeTruthy();
  });

  it("says nothing exists only when nothing is narrowing, scope included", () => {
    // A screen's own scope (which pipeline's board is being read) narrows the
    // rows, and no button here can undo it. Switching to a pipeline with no
    // deals said "no deals yet" about a workspace full of them.
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        scopeKey="pipeline-2"
      />,
    );
    expect(screen.queryByText(/No rows yet/)).toBeNull();
    expect(screen.getByText(/No rows match these filters/)).toBeTruthy();
    // And no verb, because there is nothing here that could clear it. A Clear
    // that cleared nothing is a control that does nothing.
    expect(
      screen.queryByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeNull();
  });

  it("names a likelier cause under the none-yet copy", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        emptyNote="No owner here maps to a workspace user."
      />,
    );
    expect(
      screen.getByText("No owner here maps to a workspace user."),
    ).toBeTruthy();
  });

  const ownerChips: readonly ListChip[] = [
    {
      key: "owner_id",
      label: "Owner",
      allLabel: "Any owner",
      options: [{ value: "u-9", label: "Me" }],
    },
  ];

  it("names it under the no-matches copy too, since a narrowing can be the cause", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        emptyNote="Your role cannot narrow this list by Owner."
        chips={ownerChips}
        chosen={{ owner_id: "u-9" }}
      />,
    );
    expect(screen.getByText("No rows match these filters.")).toBeTruthy();
    expect(
      screen.getByText("Your role cannot narrow this list by Owner."),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeTruthy();
  });

  it("puts a caller's empty line in place of the no-matches copy and its Clear", () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        emptyLine="You own no rows."
        chips={ownerChips}
        chosen={{ owner_id: "u-9" }}
      />,
    );
    expect(screen.getByText("You own no rows.")).toBeTruthy();
    expect(screen.queryByText(/match these filters/)).toBeNull();
    expect(
      screen.queryByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeNull();
  });
});

// A search narrowed to no rows may get rows back with the next keystroke, so
// its count line stays. A list with nothing to narrow and no rows drops it.
describe("the count line", () => {
  const table = (search: string, rows: number) => (
    <ListTable
      rows={testRows(rows)}
      columns={columns}
      rowKey={(row) => row.id}
      unit="rows"
      search={{ value: search, answered: search, onChange: () => undefined }}
    />
  );

  it("holds its line for a narrowed list with no rows, and only then", () => {
    const { container, rerender } = render(table("", 3));
    const line = () => container.querySelector(".lt-count");
    expect(line()).toHaveTextContent("1-3 of 3");

    rerender(<LocaleProvider initial="en">{table("zzqx", 0)}</LocaleProvider>);
    expect(line()).toBeEmptyDOMElement();

    rerender(<LocaleProvider initial="en">{table("", 0)}</LocaleProvider>);
    expect(line()).toBeNull();
  });
});
