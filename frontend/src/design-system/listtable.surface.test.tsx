/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListChip, ListTable, type ListView } from "./listtable";
import { pagerSlots } from "./listtable.paging";
import { columns, render, testRows } from "./listtable.testkit";
import { pickOption } from "./select-testing";

afterEach(cleanup);

// A header whose verbs fold into one menu must behave like the row of verbs it
// replaced. By default the menu defers its children to the first open. That is
// right per row and wrong here. A verb that reads its opening state once, at
// mount, is dead until somebody presses the menu. `#/deals/new` is such an
// address: its whole meaning is "the create form is open". Below this width it
// opened nothing, and pressing the menu then opened a form nobody asked for.
function MountReporter({ onMount }: Readonly<{ onMount: () => void }>) {
  // A verb that does its work at MOUNT, which is the shape `CreateAction`'s
  // `startOpen` has: read once in `useState`, so a late mount reads it late.
  useEffect(() => {
    onMount();
  }, [onMount]);
  return <button type="button">New thing</button>;
}

describe("views", () => {
  it("switching a view reports its index and applies its sort and filters", async () => {
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

    await userEvent.click(screen.getByRole("button", { name: "New leads" }));
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
      await screen.findByRole("button", { name: "More actions" }),
    ).toBeTruthy();
    // And the verb is mounted anyway. The check is on the mount, not on the
    // button being visible, because the fold sets `hidden` on it. Visibility is
    // the wrong question and would pass on a dead control.
    expect(mounted).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
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
      screen.getByRole("combobox", { name: "Rows per page" }),
      "50 per page",
    );
    // The page size is the SERVER's page size: the table reports the choice and
    // the caller re-asks with it. A table that resliced its buffer here is what
    // made a list say "1-25 of 50 loaded so far".
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
    const picker = screen.getByRole("combobox", { name: "Rows per page" });
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
    expect(pagerSlots(1, 6)).toEqual(["room", "room", 1, 2, 3, "gap"]);
    expect(pagerSlots(4, 6)).toEqual([1, "gap", 3, 4, 5, "gap"]);
    expect(pagerSlots(6, 6)).toEqual([1, "gap", 4, 5, 6, "room"]);
  });

  it("marks skipped pages only, so a gap never stands for a page nobody fetched", () => {
    // Every page in hand is on the strip, so nothing is being held back. A gap
    // here would read as hidden pages rather than unfetched ones, which is
    // Next's to say.
    expect(pagerSlots(2, 3)).toEqual(["room", "room", 1, 2, 3, "room"]);
    expect(pagerSlots(1, 2)).toEqual(["room", "room", 1, 2, "room", "room"]);
  });

  it("holds one width so Next stays where the reader clicked it", () => {
    for (const [current, lastPage] of [
      [1, 1],
      [1, 3],
      [2, 6],
      [4, 6],
      [6, 6],
      [9, 40],
    ]) {
      expect(pagerSlots(current, lastPage)).toHaveLength(6);
    }
  });

  it("reaches page one from the strip rather than by walking Prev", async () => {
    const user = userEvent.setup();
    const { container } = render(
      <ListTable
        rows={testRows(100)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const slots = () => container.querySelectorAll(".lt-pager > *").length;
    const atFirstPage = slots();

    await user.click(screen.getByRole("button", { name: "Page 3" }));
    // The room a gap would take is rendered, not merely counted, so the strip
    // holds its width and Next stays where the reader clicked it.
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
    expect(screen.getByText(/1 to 25 of 60 rows, sorted by Name/)).toBeTruthy();
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

  it("closes the filter menu on Escape and puts focus back on its trigger", async () => {
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
    const trigger = screen.getByRole("button", { name: "Filter" });
    await user.click(trigger);
    expect(screen.getByRole("group", { name: "Filter" })).toBeTruthy();

    await user.keyboard("{Escape}");

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(trigger);
  });

  it("closes the Display menu on Escape too, which keeps its own open state", async () => {
    const user = userEvent.setup();
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const trigger = screen.getByRole("button", { name: "Display" });
    await user.click(trigger);
    expect(trigger.getAttribute("aria-expanded")).toBe("true");

    await user.keyboard("{Escape}");

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(trigger);
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
    expect(screen.getByRole("button", { name: "Clear filters" })).toBeTruthy();
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
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
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
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
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

  it("names it under the no-matches copy too, since a narrowing can be the cause", () => {
    // The case the prop was written for is a narrowed one: a "Mine" view for a
    // reader who owns nothing. Drawn only over the unnarrowed line, that note
    // never appeared at all. The caller knows which emptiness a note explains.
    // A caller whose note would blame the data source for the reader's own dial
    // passes none.
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        emptyNote="You own no rows."
        chips={[
          {
            key: "owner_id",
            label: "Owner",
            allLabel: "Any owner",
            options: [{ value: "u-9", label: "Me" }],
          },
        ]}
        chosen={{ owner_id: "u-9" }}
      />,
    );
    expect(screen.getByText("No rows match these filters.")).toBeTruthy();
    expect(screen.getByText("You own no rows.")).toBeTruthy();
    // Two different offers: one undoes every narrowing, the screen's own note
    // usually undoes one.
    expect(screen.getByRole("button", { name: "Clear filters" })).toBeTruthy();
  });
});

describe("phone card layout hooks", () => {
  it("labels every non-identity cell with its column header, and leaves the identity cell and table roles intact", () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );

    expect(screen.getByRole("table")).toBeTruthy();
    const [headerRow, dataRow] = screen.getAllByRole("row");
    expect(within(headerRow).getAllByRole("columnheader")).toHaveLength(
      columns.length,
    );

    const cells = within(dataRow).getAllByRole("cell");
    expect(cells).toHaveLength(columns.length);
    const [identity, value, note, region] = cells;
    expect(identity.hasAttribute("data-label")).toBe(false);
    expect(value.getAttribute("data-label")).toBe("Value");
    expect(note.getAttribute("data-label")).toBe("Note");
    expect(region.getAttribute("data-label")).toBe("Region");
  });
});
