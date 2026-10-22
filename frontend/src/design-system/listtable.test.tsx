/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type ListChip, ListTable } from "./listtable";
import { columns, render, testRows } from "./listtable.testkit";

// The list surface (design-system/listtable.tsx) on its own props: the query
// dials it exposes (sort, chips, views, paging) and the presentation state it
// owns itself (column visibility, density). listquery.test.tsx covers the
// server-query binding on top of this — this file proves the surface alone.

afterEach(cleanup);

// The sort dial names the order in force ("Sort: Name"), so a lookup pinned to
// the bare word only found it while the list was unsorted. Anchored, because a
// column header's own control is named "Sort by <column>".
const SORT_DIAL = /^Sort(:|$)/;

describe("sorting", () => {
  it("clicking a sortable header requests that field, ascending first", async () => {
    const onChange = vi.fn();
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Sort by Name" }));
    expect(onChange).toHaveBeenCalledWith("name");
  });

  it("clicking the already-sorted header again sends the descending field", async () => {
    function Harness() {
      const [value, setValue] = useState("name");
      return (
        <ListTable
          rows={testRows(1)}
          columns={columns}
          rowKey={(row) => row.id}
          unit="rows"
          sort={{ value, onChange: setValue }}
        />
      );
    }
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: "Sort by Name" }));
    expect(
      screen
        .getByRole("columnheader", { name: /Name/ })
        .getAttribute("aria-sort"),
    ).toBe("descending");
  });

  it("a numeric column's first click sorts descending", async () => {
    const onChange = vi.fn();
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange }}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Sort by Value" }),
    );
    expect(onChange).toHaveBeenCalledWith("-value");
  });

  it("a column with no sort field renders an inert header", () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange: () => {} }}
      />,
    );
    const header = screen.getByRole("columnheader", { name: "Note" });
    expect(within(header).queryByRole("button")).toBeNull();
  });

  it("omitting the sort control makes every header inert", () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(screen.queryByRole("button", { name: /Sort by/ })).toBeNull();
  });
});

describe("the sort menu", () => {
  /** The menu's own entries, so the Display menu's "Value" is never one of them. */
  function sortMenu() {
    return within(screen.getByRole("group", { name: "Sort by" }));
  }

  it("offers every orderable column and nothing else", async () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange: () => {} }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    const menu = sortMenu();
    expect(menu.getByRole("radio", { name: "Name" })).toBeTruthy();
    expect(menu.getByRole("radio", { name: "Value" })).toBeTruthy();
    // Note and Region name no server sort field, so ordering by them is
    // something the API cannot do and the menu must not offer.
    expect(menu.queryByRole("radio", { name: "Note" })).toBeNull();
    expect(menu.queryByRole("radio", { name: "Region" })).toBeNull();
  });

  it("still offers a column the reader has hidden", async () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange: () => {} }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Display" }));
    await userEvent.click(
      within(screen.getByRole("group", { name: "Display" })).getByRole(
        "checkbox",
        { name: "Value" },
      ),
    );
    expect(screen.queryByRole("columnheader", { name: "Value" })).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    expect(sortMenu().getByRole("radio", { name: "Value" })).toBeTruthy();
  });

  it("presses the same direction a header press would", async () => {
    const onChange = vi.fn();
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    await userEvent.click(sortMenu().getByRole("radio", { name: "Name" }));
    expect(onChange).toHaveBeenCalledWith("name");

    await userEvent.click(sortMenu().getByRole("radio", { name: "Value" }));
    expect(onChange).toHaveBeenLastCalledWith("-value");
  });

  it("flips the direction of the attribute already sorted, and says which way", async () => {
    function Harness() {
      const [value, setValue] = useState("name");
      return (
        <ListTable
          rows={testRows(1)}
          columns={columns}
          rowKey={(row) => row.id}
          unit="rows"
          sort={{ value, onChange: setValue }}
        />
      );
    }
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    const entry = sortMenu().getByRole("radio", { name: /^Name/ });
    expect(entry).toBeChecked();
    expect(entry.closest("label")?.textContent).toContain("ascending");

    await userEvent.click(entry);
    expect(
      sortMenu().getByRole("radio", { name: /^Name/ }).closest("label")
        ?.textContent,
    ).toContain("descending");
  });

  // A sort arrives from three places a reader did not press — a saved view, a
  // column header, an address they pasted — so the dial is the one control where
  // all three are visible. Labelled only "Sort" it made them open it to find
  // out.
  it("names the order in force on the dial itself", async () => {
    const { rerender } = render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange: () => {} }}
      />,
    );
    expect(
      screen.getByRole("button", { name: SORT_DIAL }),
    ).toHaveAccessibleName("Sort");

    rerender(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "-value", onChange: () => {} }}
      />,
    );
    // Which WAY the list runs is the dial's other half, carried in text only a
    // screen reader hears, so the name says the column and the order together.
    expect(
      screen.getByRole("button", { name: SORT_DIAL }),
    ).toHaveAccessibleName("Sort: Value descending");
  });

  // ONE order at a time, so the entries are RADIOS: a box is the shape of a set
  // a reader adds to, and five of them over five orderings promised a
  // combination this list cannot be in.
  it("marks the order in force without offering a set to build", async () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "name", onChange: () => {} }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    const menu = sortMenu();

    expect(menu.getByRole("radio", { name: /^Name/ })).toBeChecked();
    expect(menu.getByRole("radio", { name: "Value" })).not.toBeChecked();
    expect(menu.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("offers the server's own order, which is a state a saved view can ask for", async () => {
    const onChange = vi.fn();
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "name", onChange }}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: SORT_DIAL }));
    const fallback = sortMenu().getByRole("radio", { name: "Default order" });
    expect(fallback).not.toBeChecked();
    await userEvent.click(fallback);
    expect(onChange).toHaveBeenCalledWith("");
  });

  it("is not drawn for a list the server cannot order", () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={[{ key: "note", header: "Note", cell: () => "-" }]}
        rowKey={(row) => row.id}
        unit="rows"
        sort={{ value: "", onChange: () => {} }}
      />,
    );
    expect(screen.queryByRole("button", { name: SORT_DIAL })).toBeNull();
  });

  it("is not drawn without a sort control at all", () => {
    render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(screen.queryByRole("button", { name: SORT_DIAL })).toBeNull();
  });
});

describe("filter chips", () => {
  const chips: readonly ListChip[] = [
    {
      key: "status",
      label: "Status",
      allLabel: "All statuses",
      options: [
        { value: "new", label: "New" },
        { value: "won", label: "Won" },
      ],
    },
  ];

  it("picking a chip option calls onChipChange with the value, and Delete filter clears it", async () => {
    const onChipChange = vi.fn();
    const { rerender } = render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        onChipChange={onChipChange}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    await userEvent.click(screen.getByRole("button", { name: "Status" }));
    await userEvent.click(screen.getByRole("radio", { name: "New" }));
    expect(onChipChange).toHaveBeenCalledWith("status", "new");

    rerender(
      <LocaleProvider initial="en">
        <ListTable
          rows={[]}
          columns={columns}
          rowKey={(row) => row.id}
          unit="rows"
          chips={chips}
          chosen={{ status: "new" }}
          onChipChange={onChipChange}
        />
      </LocaleProvider>,
    );
    await userEvent.click(
      screen.getByRole("button", {
        name: "More actions for the Status filter",
      }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Delete filter" }),
    );
    expect(onChipChange).toHaveBeenCalledWith("status", "");
  });
});

describe("column widths", () => {
  /** The `<col>` width the table hands column `key`, as the caller reads it. */
  function widthOf(container: HTMLElement, index: number): string {
    const col = container.querySelectorAll(".lt-table col")[index];
    if (!(col instanceof HTMLTableColElement)) {
      throw new Error(`no col at index ${index}`);
    }
    return col.style.width;
  }

  // The defect this pins: the minimums used to be summed into the table's own
  // min-width and never applied to a column, so a column whose share came out
  // under its minimum took the share and cut its content off. jsdom reports a
  // zero-width body, which is the narrowest case there is — every column is
  // then at its floor, and its floor is what has to be on the element.
  it("never hands a column less than the minimum its kind declares", () => {
    const { container } = render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    // identity, numeric, standard, standard — COLUMN_SIZES' own floors.
    expect(widthOf(container, 0)).toBe("200px");
    expect(widthOf(container, 1)).toBe("110px");
    expect(widthOf(container, 2)).toBe("130px");
    expect(widthOf(container, 3)).toBe("130px");
  });

  // A verbs column is sized by its buttons, so it takes its width outright
  // instead of a share: a fraction of a 654px settings column is not a width
  // "Produkt archivieren" fits in, and a half-drawn verb is an unusable one.
  it("gives a verbs column its own width, whatever the table's is", () => {
    const { container } = render(
      <ListTable
        rows={testRows(1)}
        columns={[
          ...columns,
          { key: "actions", header: "Actions", cell: () => "-", verbs: true },
        ]}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    expect(widthOf(container, 4)).toBe("320px");
    // And the columns beside it keep their own floors rather than being
    // squeezed to pay for it.
    expect(widthOf(container, 0)).toBe("200px");
    expect(widthOf(container, 1)).toBe("110px");
  });

  // The trailing column's divider is cleared by the stylesheet — there is no
  // edge there to grab, and a grip drawn against nothing is the fault this
  // pins. Every column BEFORE it keeps one.
  it("puts a grip on every column edge except the trailing one", () => {
    const { container } = render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const headers = container.querySelectorAll(".lt-table thead th");
    expect(headers).toHaveLength(columns.length);
    for (const [index, header] of headers.entries()) {
      expect(Boolean(header.querySelector(".lt-grip"))).toBe(
        index < columns.length - 1,
      );
    }
  });

  // A drag that highlighted the rows it passed over answered "did I grab the
  // right thing" with "no". The whole grid wears the gesture until it ends,
  // and a cancelled pointer ends it as surely as a released one — otherwise
  // the table stays dressed for a drag nothing is driving.
  it("dresses the grid for the drag and undresses it when the drag ends", () => {
    const { container } = render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const table = container.querySelector(".lt-table");
    const grip = container.querySelector(".lt-grip");
    if (!(table instanceof HTMLElement) || !(grip instanceof HTMLElement)) {
      throw new Error("no resizable header rendered");
    }
    // jsdom has no pointer capture, so the grip's own calls need a stand-in.
    grip.setPointerCapture = () => undefined;
    grip.releasePointerCapture = () => undefined;

    expect(table.className).not.toContain("is-resizing");
    fireEvent.pointerDown(grip, { clientX: 100, pointerId: 1 });
    expect(table.className).toContain("is-resizing");
    expect(grip.className).toContain("is-dragging");

    fireEvent.pointerCancel(grip, { pointerId: 1 });
    expect(table.className).not.toContain("is-resizing");
    expect(grip.className).not.toContain("is-dragging");
  });
});

describe("the frozen column's edge", () => {
  it("only casts a shadow once columns have scrolled under it", () => {
    const { container } = render(
      <ListTable
        rows={testRows(1)}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
      />,
    );
    const scroller = container.querySelector(".lt-scroll");
    if (!(scroller instanceof HTMLElement)) {
      throw new Error("the table has no scrolling body");
    }
    expect(scroller.className).not.toContain("shifted");

    scroller.scrollLeft = 120;
    fireEvent.scroll(scroller);
    expect(scroller.className).toContain("shifted");

    scroller.scrollLeft = 0;
    fireEvent.scroll(scroller);
    expect(scroller.className).not.toContain("shifted");
  });
});
