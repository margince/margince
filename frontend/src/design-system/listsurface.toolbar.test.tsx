/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { precedes } from "../testing/domorder";
import {
  openView,
  openViewStep,
  viewButton,
} from "../testing/listview.testkit";
import { Heading } from "./heading";
import {
  CountLine,
  type ListChip,
  ListSurface,
  type SortOption,
} from "./listsurface";
import { Modal } from "./modal";

// The toolbar as a reader meets it: the search, View and its panel's rows and
// steps, and the applied filters under the search.

afterEach(cleanup);

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

const CHIPS: readonly ListChip[] = [
  {
    key: "status",
    label: "Status",
    allLabel: "Any status",
    options: [
      { value: "open", label: "Open" },
      { value: "won", label: "Won" },
    ],
  },
  {
    key: "owner",
    label: "Owner",
    allLabel: "Any owner",
    options: [{ value: "u1", label: "Ada" }],
  },
];

const SORT_OPTIONS: readonly SortOption[] = [
  { field: "name", label: "Name" },
  { field: "created_at", label: "Created" },
];

/** The surface with every dial it can carry, driving its own state. */
function Surface({
  chips = CHIPS,
  initialSort = "",
  opensOn,
  initialFilters = {},
  changed = false,
  onChipChange,
}: Readonly<{
  chips?: readonly ListChip[];
  initialSort?: string;
  /** The order the list opens on (`sort.initial`). */
  opensOn?: string;
  initialFilters?: Readonly<Record<string, string>>;
  changed?: boolean;
  onChipChange?: (key: string, value: string) => void;
}>) {
  const [sort, setSort] = useState(initialSort);
  const [chosen, setChosen] = useState(initialFilters);
  const [search, setSearch] = useState("");
  const [archived, setArchived] = useState(false);
  return (
    <ListSurface
      search={{ value: search, onChange: setSearch }}
      sort={{ value: sort, onChange: setSort, initial: opensOn }}
      sortOptions={SORT_OPTIONS}
      chips={chips}
      chosen={chosen}
      onChipChange={(key, value) => {
        onChipChange?.(key, value);
        setChosen((prev) => ({ ...prev, [key]: value }));
      }}
      archived={{ checked: archived, onChange: setArchived }}
      changed={changed}
      saveView={<button type="button">Save view</button>}
    >
      <p>rows</p>
    </ListSurface>
  );
}

it("holds a search and one View button, and nothing else", () => {
  render(<Surface />);

  const tools = screen
    .getByRole("searchbox", { name: "Search" })
    .closest(".lt-tools");
  if (!(tools instanceof HTMLElement)) throw new Error("no toolbar");
  expect(within(tools).getAllByRole("button")).toHaveLength(1);
  expect(within(tools).queryByRole("checkbox")).toBeNull();
  expect(within(tools).queryByRole("switch")).toBeNull();
});

it("names on View how many filters apply and that the list left its view", async () => {
  render(<Surface initialFilters={{ status: "open", owner: "u1" }} changed />);

  const view = await viewButton();
  expect(view).toHaveAccessibleName("View, 2 filters, changed");
  expect(view.querySelector(".optioncount")).toHaveTextContent("2");

  cleanup();
  render(<Surface />);
  expect(await viewButton()).toHaveAccessibleName("View");
});

it("reads every dial's value on its row, in the order the panel holds them", async () => {
  const user = userEvent.setup();
  render(
    <Surface initialSort="-created_at" initialFilters={{ status: "open" }} />,
  );
  const panel = await openView(user);

  const rows = within(panel)
    .getAllByRole("button")
    .filter((row) => row.dataset.viewRow !== undefined)
    .map((row) => row.getAttribute("aria-label"));
  expect(rows).toEqual([
    "Filters, Status, changed",
    "Sort, Created, descending, changed",
  ]);
  const archived = within(panel).getByRole("button", { name: "Show archived" });
  const save = within(panel).getByRole("button", { name: "Save view" });
  expect(precedes(archived, save)).toBe(true);
});

it("flips the sort from its step and says so on the row", async () => {
  const user = userEvent.setup();
  render(<Surface initialSort="-created_at" />);

  const step = await openViewStep(user, "Sort");
  await user.click(within(step).getByRole("radio", { name: /^Created/ }));
  await user.click(
    within(step).getByRole("button", { name: "Sort, back to view options" }),
  );

  expect(
    screen.getByRole("button", {
      name: "Sort, Created, ascending, changed",
    }),
  ).toBeInTheDocument();
});

// It widens the list as a filter narrows it, so it is a pressed button and
// never a switch.
it("presses Show archived on and off from its row", async () => {
  const user = userEvent.setup();
  render(<Surface />);
  const panel = await openView(user);
  const archived = within(panel).getByRole("button", { name: "Show archived" });
  expect(archived).toHaveAttribute("aria-pressed", "false");
  expect(archived).toHaveTextContent("Off");

  await user.click(archived);
  expect(archived).toHaveAttribute("aria-pressed", "true");
  expect(archived).toHaveTextContent("On");

  await user.click(archived);
  expect(archived).toHaveAttribute("aria-pressed", "false");
});

// A row that opens a subset says so, and the level it opens says how to leave.
it("steps into an attribute and back out to its row", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  const menu = await openViewStep(user, "Filters");
  await user.click(within(menu).getByRole("button", { name: "Status" }));
  // The visible word first: a reader speaking what they see says "Status".
  const back = screen.getByRole("button", {
    name: "Status, back to all filters",
  });
  expect(back).toHaveFocus();
  expect(screen.queryByPlaceholderText("Search attributes")).toBeNull();

  await user.click(back);
  expect(screen.getByRole("button", { name: "Status" })).toHaveFocus();
  expect(screen.getByPlaceholderText("Search attributes")).toBeInTheDocument();
});

// The step replaced the row the reader pressed, so Back hands focus to it.
it("hands focus to the step's way back, and back to the row that opened it", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await openViewStep(user, "Sort");
  const back = screen.getByRole("button", {
    name: "Sort, back to view options",
  });
  expect(back).toHaveFocus();

  await user.click(back);
  expect(screen.getByRole("button", { name: /^Sort,/ })).toHaveFocus();
});

it("applies a picked value, returns to its row, and stands the filter under the search", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  const menu = await openViewStep(user, "Filters");
  await user.click(within(menu).getByRole("button", { name: "Status" }));
  await user.click(screen.getByRole("radio", { name: "Open" }));

  expect(
    screen.getByRole("button", { name: "Status, Open, changed" }),
  ).toHaveFocus();
  const applied = screen.getByRole("group", { name: "Status: Open" });
  expect(
    precedes(screen.getByRole("searchbox", { name: "Search" }), applied),
  ).toBe(true);
  // Adding lives in the panel: the row of applied filters offers no "+".
  expect(screen.queryByRole("button", { name: "Add filter" })).toBeNull();
});

it("lists every attribute, an applied one with its value and a dot", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open" }} />);

  const menu = await openViewStep(user, "Filters");
  expect(
    within(menu).getByRole("button", { name: "Status, Open, changed" }),
  ).toBeInTheDocument();
  expect(
    within(menu).getByRole("button", { name: "Owner" }),
  ).toBeInTheDocument();
});
it("draws no applied-filter row while nothing applies", () => {
  const { container } = render(<Surface />);
  expect(container.querySelector(".lt-applied")).toBeNull();
});

// A non-dial verb stays on the row; it is not a dial and has no row to live in.
it("stands a screen's own verb after View", async () => {
  render(
    <ListSurface
      search={{ value: "", onChange: () => undefined }}
      sort={{ value: "", onChange: () => undefined }}
      sortOptions={SORT_OPTIONS}
      tools={<button type="button">Coverage</button>}
    >
      <p>rows</p>
    </ListSurface>,
  );

  expect(
    precedes(
      await viewButton(),
      screen.getByRole("button", { name: "Coverage" }),
    ),
  ).toBe(true);
});

it("closes the whole panel on Escape, from any step, and hands focus to View", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  const menu = await openViewStep(user, "Filters");
  await user.click(within(menu).getByRole("button", { name: "Status" }));
  await user.keyboard("{Escape}");

  const view = await viewButton();
  expect(view).toHaveAttribute("aria-expanded", "false");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(view).toHaveFocus();
});

// One overlay at a time, whichever opened last.
it("closes the panel when an applied filter's segment opens, and the other way about", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open" }} />);
  const value = () =>
    within(screen.getByRole("group", { name: "Status: Open" })).getByRole(
      "button",
      { name: "Open" },
    );

  await openView(user);
  await user.click(value());
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(value()).toHaveAttribute("aria-expanded", "true");

  await user.click(await viewButton());
  expect(await viewButton()).toHaveAttribute("aria-expanded", "true");
  expect(value()).toHaveAttribute("aria-expanded", "false");
});

it("leaves Escape to a dialog raised over the open panel", async () => {
  const onDialogClose = vi.fn();
  const page = (dialogOpen: boolean) => (
    <LocaleProvider initial="en">
      <Surface />
      <Modal
        open={dialogOpen}
        onClose={onDialogClose}
        labelledBy="edit"
        intent="form"
      >
        <Heading size="large" id="edit">
          Edit deal
        </Heading>
      </Modal>
    </LocaleProvider>
  );
  const user = userEvent.setup();
  const { rerender } = rtlRender(page(false));
  await openView(user);
  rerender(page(true));

  await user.keyboard("{Escape}");

  expect(onDialogClose).toHaveBeenCalledOnce();
  expect(await viewButton()).toHaveAttribute("aria-expanded", "true");
});

// The count gives way on screen, never in the text: however narrow the row, the
// whole sentence is there for a screen reader, and it reads before the verbs it
// sits beside.
it("keeps the whole count sentence, read before the header's verbs", () => {
  render(
    <ListSurface
      count="1 to 25 of 1,312 companies loaded, sorted by Last updated"
      action={<button type="button">New company</button>}
    >
      <p>rows</p>
    </ListSurface>,
  );

  const count = screen.getByText(
    "1 to 25 of 1,312 companies loaded, sorted by Last updated",
  );
  expect(
    precedes(count, screen.getByRole("button", { name: "New company" })),
  ).toBe(true);
});

// Whether the line is drawn follows the count's state. Its height while
// pending is measured in a browser (List surface, Count holds its line).
it("draws the count line as it last settled while a count is pending", () => {
  const surface = (count: ReactNode, pending: boolean) => (
    <LocaleProvider initial="en">
      <ListSurface
        views={[{ label: "All" }]}
        count={count}
        countPending={pending}
      >
        <p>rows</p>
      </ListSurface>
    </LocaleProvider>
  );
  const { container, rerender } = rtlRender(surface(undefined, true));
  const line = () => container.querySelector(".lt-count");
  expect(line()).toBeEmptyDOMElement();

  rerender(surface("1 to 25 of 40 leads", false));
  expect(line()).toHaveTextContent("1 to 25 of 40 leads");
  rerender(surface(undefined, true));
  expect(line()).toBeEmptyDOMElement();

  rerender(surface(undefined, false));
  expect(line()).toBeNull();
  rerender(surface(undefined, true));
  expect(line()).toBeNull();
});

it("says nothing for a settled list with no rows and no order", () => {
  render(<CountLine unit="leads" first={1} last={0} total={0} />);
  expect(document.body).toHaveTextContent(/^$/);
  cleanup();
  render(
    <CountLine unit="leads" first={1} last={0} total={0} sortedBy="Name" />,
  );
  expect(document.body).toHaveTextContent("Sorted by Name");
});

it("names what the counted rows belong to, before the order", () => {
  render(
    <ListSurface
      count={
        <CountLine
          unit="deals"
          first={1}
          last={2}
          total={2}
          scope="Sales"
          sortedBy="Value"
        />
      }
    >
      <p>rows</p>
    </ListSurface>,
  );
  expect(
    screen.getByText("1 to 2 of 2 deals in Sales, sorted by Value"),
  ).toBeInTheDocument();
});

it("changes an applied filter in place, under the same key and condition", async () => {
  const user = userEvent.setup();
  const writes: [string, string][] = [];
  render(
    <Surface
      initialFilters={{ status: "open" }}
      onChipChange={(key, value) => writes.push([key, value])}
    />,
  );

  const menu = await openViewStep(user, "Filters");
  await user.click(
    within(menu).getByRole("button", { name: "Status, Open, changed" }),
  );
  const panel = within(await openView(user));
  const step = within(panel.getByRole("group", { name: "Status" }));
  expect(step.getByRole("radio", { name: "Open" })).toBeChecked();
  await user.click(step.getByRole("radio", { name: "Won" }));

  expect(writes).toEqual([["status", "won"]]);
  expect(
    screen.getByRole("button", { name: "Status, Won, changed" }),
  ).toHaveFocus();
  const applied = screen.getByRole("group", { name: "Status: Won" });
  expect(
    within(applied).getByRole("button", { name: "is" }),
  ).toBeInTheDocument();
});

const CLOSE_DATE: ListChip = {
  key: "close",
  label: "Close date",
  allLabel: "Any date",
  options: [],
  editor: () => <p>Pick a range</p>,
  appliedName: "This month",
};

it.each([
  ["its any option", "status", "Status, Open, changed", "Any status"],
  ["Delete filter", "status", "Status, Open, changed", "Delete filter"],
  [
    "Delete filter, with no any option",
    "close",
    "Close date, This month, changed",
    "Delete filter",
  ],
])(
  "takes a filter off from its value step by %s",
  async (_how, key, row, press) => {
    const user = userEvent.setup();
    const writes: [string, string][] = [];
    render(
      <Surface
        chips={[...CHIPS, CLOSE_DATE]}
        initialFilters={{ status: "open", close: "2026-11" }}
        onChipChange={(changed, value) => writes.push([changed, value])}
      />,
    );

    const menu = await openViewStep(user, "Filters");
    await user.click(within(menu).getByRole("button", { name: row }));
    const panel = within(await openView(user));
    const step = panel.getByRole("group", { name: row.split(",")[0] });
    await user.click(
      within(step).getByRole(press === "Delete filter" ? "button" : "radio", {
        name: press,
      }),
    );

    expect(writes).toEqual([[key, ""]]);
    const label = row.split(",")[0];
    expect(screen.getByRole("button", { name: label })).toHaveFocus();
    expect(
      screen.queryByRole("group", { name: new RegExp(`^${label}:`) }),
    ).toBeNull();
  },
);

it("puts a value step's search above its options and its any option", async () => {
  const user = userEvent.setup();
  const team: ListChip = {
    key: "team",
    label: "Team",
    allLabel: "Any team",
    options: [{ value: "t1", label: "North" }],
    filterable: true,
  };
  render(<Surface chips={[team]} />);

  const menu = await openViewStep(user, "Filters");
  await user.click(within(menu).getByRole("button", { name: "Team" }));
  const step = within(await openView(user)).getByRole("group", {
    name: "Team",
  });

  expect(
    precedes(
      within(step).getByRole("searchbox", { name: "Search Team values" }),
      within(step).getByRole("radio", { name: "Any team" }),
    ),
  ).toBe(true);
  // A filter that does not apply has nothing to delete.
  expect(
    within(step).queryByRole("button", { name: "Delete filter" }),
  ).toBeNull();
});

// "Default order" is the server's own. A shared link with no sort arrives on
// it, and a row that read "Default order, changed" would contradict itself.
it.each([
  ["the server's order", "", "Sort, Default order"],
  ["the order it opens on", "-created_at", "Sort, Created, descending"],
  ["another named order", "name", "Sort, Name, ascending, changed"],
])(
  "marks Sort changed only on a named order it did not open on: %s",
  async (_case, value, name) => {
    const user = userEvent.setup();
    render(<Surface initialSort={value} opensOn="-created_at" />);
    const panel = await openView(user);
    expect(
      within(panel).getByRole("button", { name: /^Sort,/ }),
    ).toHaveAccessibleName(name);
  },
);

// The menu that took the pick or the delete closes. Focus must not fall to the
// page: it stays on the chip, goes to the next one, or goes to View.
it("keeps focus on a chip's value after a pick from its menu", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open" }} />);
  const applied = screen.getByRole("group", { name: "Status: Open" });
  await user.click(within(applied).getByRole("button", { name: "Open" }));
  await user.click(screen.getByRole("radio", { name: "Won" }));

  const changed = screen.getByRole("group", { name: "Status: Won" });
  expect(within(changed).getByRole("button", { name: "Won" })).toHaveFocus();
});

it("hands focus to the next chip after a delete, and to View once none is left", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open", owner: "u1" }} />);
  const deleteFrom = async (filter: string) => {
    await user.click(
      screen.getByRole("button", {
        name: `More actions for the ${filter} filter`,
      }),
    );
    const menu = screen
      .getAllByRole("group", { name: filter })
      .find((group) => group.classList.contains("open"));
    if (menu === undefined) throw new Error(`no open ${filter} menu`);
    await user.click(
      within(menu).getByRole("button", { name: "Delete filter" }),
    );
  };

  await deleteFrom("Status");
  const owner = screen.getByRole("group", { name: "Owner: Ada" });
  expect(within(owner).getByRole("button", { name: "Ada" })).toHaveFocus();

  await deleteFrom("Owner");
  expect(await viewButton()).toHaveFocus();
});

// A list with no View panel draws Show archived as the panel does: one
// pressed button, never a checkbox.
it("draws a bare list's Show archived as a pressed button", async () => {
  const user = userEvent.setup();
  function Bare() {
    const [checked, setChecked] = useState(false);
    return (
      <ListSurface bare archived={{ checked, onChange: setChecked }}>
        <p>rows</p>
      </ListSurface>
    );
  }
  render(<Bare />);

  expect(screen.queryByRole("checkbox")).toBeNull();
  const archived = screen.getByRole("button", { name: "Show archived" });
  expect(archived).toHaveAttribute("aria-pressed", "false");
  await user.click(archived);
  expect(archived).toHaveAttribute("aria-pressed", "true");
});

// A short list turns bare when Show archived goes off. The panel the reader
// pressed it in stays until they close it, and focus then finds the switch.
it("keeps the panel open while the list turns bare under it", async () => {
  const user = userEvent.setup();
  function ShortList() {
    const [checked, setChecked] = useState(true);
    return (
      <ListSurface
        bare={!checked}
        search={{ value: "", onChange: () => undefined }}
        archived={{ checked, onChange: setChecked }}
      >
        <p>rows</p>
      </ListSurface>
    );
  }
  render(<ShortList />);

  const panel = await openView(user);
  await user.click(
    within(panel).getByRole("button", { name: "Show archived" }),
  );
  expect(screen.getByRole("dialog")).toBe(panel);
  expect(
    within(panel).getByRole("button", { name: "Show archived" }),
  ).toHaveAttribute("aria-pressed", "false");

  await user.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.queryByRole("searchbox")).toBeNull();
  expect(screen.getByRole("button", { name: "Show archived" })).toHaveFocus();
});

// A failed read the dials depend on is said on the toolbar, outside the
// panel, so the reader sees it without opening View.
it("stands a notice on its own line under the search, outside View", () => {
  render(
    <ListSurface
      search={{ value: "", onChange: () => undefined }}
      sort={{ value: "", onChange: () => undefined }}
      sortOptions={SORT_OPTIONS}
      notice={<p role="alert">Saved views did not load</p>}
    >
      <p>rows</p>
    </ListSurface>,
  );

  const notice = screen.getByRole("alert");
  expect(notice.closest(".lt-tools")).not.toBeNull();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(
    precedes(screen.getByRole("searchbox", { name: "Search" }), notice),
  ).toBe(true);
});
