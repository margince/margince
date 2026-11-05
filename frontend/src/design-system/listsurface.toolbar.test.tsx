/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
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
import { rulesIn } from "../testing/css";
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

const here = dirname(fileURLToPath(import.meta.url));

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
  initialSort = "",
  initialFilters = {},
  changed = false,
}: Readonly<{
  initialSort?: string;
  initialFilters?: Readonly<Record<string, string>>;
  changed?: boolean;
}>) {
  const [sort, setSort] = useState(initialSort);
  const [chosen, setChosen] = useState(initialFilters);
  const [search, setSearch] = useState("");
  const [archived, setArchived] = useState(false);
  return (
    <ListSurface
      search={{ value: search, onChange: setSearch }}
      sort={{ value: sort, onChange: setSort }}
      sortOptions={SORT_OPTIONS}
      chips={CHIPS}
      chosen={chosen}
      onChipChange={(key, value) =>
        setChosen((prev) => ({ ...prev, [key]: value }))
      }
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
  expect(view.querySelector(".lt-vdot")).toBeTruthy();

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
    .filter((row) => row.classList.contains("lt-vrow"))
    .map((row) => row.getAttribute("aria-label"));
  expect(rows).toEqual(["Filters, Status", "Sort, Created, descending"]);
  const archived = within(panel).getByRole("button", { name: "Show archived" });
  const save = within(panel).getByRole("button", { name: "Save view" });
  expect(precedes(archived, save)).toBe(true);
});

it("flips the sort from its step and says so on the row", async () => {
  const user = userEvent.setup();
  render(<Surface initialSort="-created_at" />);

  const step = await openViewStep(user, "Sort");
  await user.click(within(step).getByRole("radio", { name: /^Created/ }));
  await user.click(within(step).getByRole("button", { name: /^Sort Back/ }));

  expect(
    screen.getByRole("button", { name: "Sort, Created, ascending" }),
  ).toBeTruthy();
});

// It widens the list as a filter narrows it, so it is a pressed button and
// never a switch.
it("presses Show archived on and off from its row", async () => {
  const user = userEvent.setup();
  render(<Surface />);
  const panel = await openView(user);
  const archived = within(panel).getByRole("button", { name: "Show archived" });
  expect(archived).toHaveAttribute("aria-pressed", "false");

  await user.click(archived);
  expect(archived).toHaveAttribute("aria-pressed", "true");

  await user.click(archived);
  expect(archived).toHaveAttribute("aria-pressed", "false");
});

// A row that opens a subset says so, and the level it opens says how to leave.
it("steps into an attribute and back out to the search it came from", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  const menu = await openViewStep(user, "Filters");
  const row = within(menu).getByRole("button", { name: "Status" });
  expect(row.querySelector(".lucide-chevron-right")).toBeTruthy();

  await user.click(row);
  // The visible word first: a reader speaking what they see says "Status".
  const back = screen.getByRole("button", {
    name: /^Status Back to all filters$/,
  });
  expect(back).toHaveFocus();
  expect(back.querySelector(".lucide-chevron-left")).toBeTruthy();
  expect(screen.queryByPlaceholderText("Search attributes")).toBeNull();

  await user.click(back);
  const search = screen.getByPlaceholderText("Search attributes");
  expect(search).toHaveFocus();
  expect(screen.getByRole("button", { name: "Owner" })).toBeTruthy();
});

// The step replaced the row the reader pressed, so Back hands focus to it.
it("hands focus to the step's way back, and back to the row that opened it", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await openViewStep(user, "Sort");
  const back = screen.getByRole("button", {
    name: /^Sort Back to view options$/,
  });
  expect(back).toHaveFocus();

  await user.click(back);
  expect(screen.getByRole("button", { name: /^Sort,/ })).toHaveFocus();
});

it("applies a picked value, returns to the rows, and stands the filter under the search", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  const menu = await openViewStep(user, "Filters");
  await user.click(within(menu).getByRole("button", { name: "Status" }));
  await user.click(screen.getByRole("radio", { name: "Open" }));

  const filters = screen.getByRole("button", { name: "Filters, Status" });
  expect(filters).toHaveFocus();
  const applied = screen.getByRole("group", { name: "Status: Open" });
  expect(
    precedes(screen.getByRole("searchbox", { name: "Search" }), applied),
  ).toBe(true);
  // Adding lives in the panel: the row of applied filters offers no "+".
  expect(screen.queryByRole("button", { name: "Add filter" })).toBeNull();
});

it("offers only the attributes not applied yet", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open" }} />);

  const menu = await openViewStep(user, "Filters");
  expect(within(menu).queryByRole("button", { name: "Status" })).toBeNull();
  expect(within(menu).getByRole("button", { name: "Owner" })).toBeTruthy();
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

// A list with nothing to count, or still loading, keeps no empty line under
// its tabs: the sheet hides an empty count, and an empty count is what draws.
it("draws no count line while there is nothing to count", () => {
  const { container, rerender } = render(
    <ListSurface
      views={[{ label: "All" }]}
      count={<CountLine unit="leads" first={1} last={0} total={0} />}
      action={<button type="button">New lead</button>}
    >
      <p>rows</p>
    </ListSurface>,
  );
  const count = () => container.querySelector(".lt-count");
  expect(count()).toBeEmptyDOMElement();

  rerender(
    <LocaleProvider initial="en">
      <ListSurface views={[{ label: "All" }]} count={false}>
        <p>rows</p>
      </ListSurface>
    </LocaleProvider>,
  );
  expect(count()).toBeEmptyDOMElement();

  const sheet = readFileSync(join(here, "listtable.css"), "utf8");
  expect(
    rulesIn(sheet).some(
      (rule) =>
        rule.selector.trim() === ".lt-count:empty" &&
        /display:\s*none/.test(rule.body),
    ),
  ).toBe(true);
});
