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
import { Heading } from "./heading";
import { type ListChip, ListSurface, type SortOption } from "./listsurface";
import { Modal } from "./modal";

// The toolbar as a reader meets it: what the sort dial SAYS without being
// opened, which triggers promise a list behind them, the order the row is read
// in, and the way back out of a filter's second step. listtable.test.tsx covers
// the table's own dials on top of this surface.

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
}: Readonly<{
  initialSort?: string;
  initialFilters?: Readonly<Record<string, string>>;
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
    >
      <p>rows</p>
    </ListSurface>
  );
}

const SORT_DIAL = /^Sort(:|$)/;

function sortDial(): HTMLElement {
  return screen.getByRole("button", { name: SORT_DIAL });
}

function filterTrigger(): HTMLElement {
  return screen.getByRole("button", { name: "Filter" });
}

// A sort arrives from three places a reader did not press — a saved view, a
// column header, an address they pasted — so which way the list runs has to be
// legible on the dial itself, and in both halves: the glyph for the reader who
// sees it and a word for the reader who does not.
it("says which way the list runs without being opened", async () => {
  const user = userEvent.setup();
  render(<Surface initialSort="-created_at" />);

  expect(sortDial()).toHaveAccessibleName("Sort: Created descending");
  expect(sortDial().querySelector(".lucide-arrow-down")).toBeTruthy();

  await user.click(sortDial());
  await user.click(screen.getByRole("radio", { name: /^Created/ }));

  expect(sortDial()).toHaveAccessibleName("Sort: Created ascending");
  expect(sortDial().querySelector(".lucide-arrow-up")).toBeTruthy();
  expect(sortDial().querySelector(".lucide-arrow-down")).toBeNull();
});

it("wears the neutral pair of arrows, and no pressed state, while unsorted", () => {
  render(<Surface />);

  expect(sortDial()).toHaveAccessibleName("Sort");
  expect(sortDial().querySelector(".lucide-arrow-down-up")).toBeTruthy();
  // The dial is not a toggle: a sort is a value it names, not a state it holds,
  // and the accent an `aria-pressed` ghost draws said the opposite.
  expect(sortDial()).toHaveClass("btn", "btn-ghost");
  expect(sortDial()).not.toHaveAttribute("aria-pressed");
});

// The caret is the product's one promise that a control opens a list, so every
// toolbar trigger that opens one wears it and the one that does not, does not.
it("carets the triggers that open a list, and only those", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  expect(sortDial().querySelector(".lt-caret")).toBeTruthy();
  expect(filterTrigger().querySelector(".lt-caret")).toBeTruthy();

  await user.click(filterTrigger());
  await user.click(screen.getByRole("button", { name: "Status" }));
  await user.click(screen.getByRole("radio", { name: "Open" }));

  const add = screen.getByRole("button", { name: "Add filter" });
  expect(add.querySelector(".lucide-plus")).toBeTruthy();
  expect(add.querySelector(".lt-caret")).toBeNull();
});

// Two halves, and the archived toggle belongs to the first: it changes WHICH
// rows are in the list, not how they are drawn, and it sat past the spacer
// among the presentation dials.
it("reads left to right as narrow-the-list, then change-the-drawing", () => {
  render(<Surface />);

  const search = screen.getByRole("searchbox", { name: "Search" });
  const archived = screen.getByLabelText("Show archived");

  expect(precedes(search, filterTrigger())).toBe(true);
  expect(precedes(filterTrigger(), archived)).toBe(true);
  expect(precedes(archived, sortDial())).toBe(true);
});

it("stands an applied filter's row ahead of the button that adds another", async () => {
  const user = userEvent.setup();
  render(<Surface initialFilters={{ status: "open" }} />);

  const row = screen.getByRole("group", { name: "Status: Open" });
  expect(
    precedes(row, screen.getByRole("button", { name: "Add filter" })),
  ).toBe(true);
  await user.click(screen.getByRole("button", { name: "Add filter" }));
  expect(screen.queryByRole("button", { name: "Status" })).toBeNull();
});

// A row that opens a subset says so, and the level it opens says how to leave.
it("steps into an attribute and back out to the search it came from", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await user.click(filterTrigger());
  const menu = screen.getByRole("group", { name: "Filter" });
  const row = within(menu).getByRole("button", { name: "Status" });
  expect(row.querySelector(".lucide-chevron-right")).toBeTruthy();

  await user.click(row);
  // The visible word first: a reader speaking what they see says "Status".
  const back = screen.getByRole("button", {
    name: /^Status Back to all filters$/,
  });
  expect(back.querySelector(".lucide-chevron-left")).toBeTruthy();
  expect(back).toHaveTextContent("Status");
  expect(screen.queryByPlaceholderText("Search attributes")).toBeNull();

  await user.click(back);
  const search = screen.getByPlaceholderText("Search attributes");
  expect(search).toHaveFocus();
  expect(screen.getByRole("button", { name: "Owner" })).toBeTruthy();
});

// A step takes the control the reader is standing on with it. Each of the
// three hands focus on rather than dropping it to the top of the document.
it("hands focus to the way back when it steps into an attribute", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await user.click(filterTrigger());
  await user.click(screen.getByRole("button", { name: "Status" }));

  expect(
    screen.getByRole("button", { name: /^Status Back to all filters$/ }),
  ).toHaveFocus();
});

it("hands focus back to the trigger when a value is picked", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await user.click(filterTrigger());
  await user.click(screen.getByRole("button", { name: "Status" }));
  await user.click(screen.getByRole("radio", { name: "Open" }));

  // The trigger the press LEFT behind: applying the first filter turns the
  // labelled button into the bare "+".
  expect(screen.getByRole("button", { name: "Add filter" })).toHaveFocus();
});

// Last on the row is the surface's own promise now, not something each screen
// remembers to put at the end of its `tools` fragment.
it("stands a saved view after the body's own dials", () => {
  render(
    <ListSurface
      sort={{ value: "", onChange: () => undefined }}
      sortOptions={SORT_OPTIONS}
      tools={<button type="button">Pipeline</button>}
      saveView={<button type="button">Save view</button>}
    >
      <p>rows</p>
    </ListSurface>,
  );

  expect(
    precedes(
      screen.getByRole("button", { name: "Pipeline" }),
      screen.getByRole("button", { name: "Save view" }),
    ),
  ).toBe(true);
});

it("closes the whole menu on Escape, from either step", async () => {
  const user = userEvent.setup();
  render(<Surface />);

  await user.click(filterTrigger());
  await user.click(screen.getByRole("button", { name: "Status" }));
  await user.keyboard("{Escape}");

  expect(filterTrigger()).toHaveAttribute("aria-expanded", "false");
  expect(
    screen.getByLabelText("Filter", { selector: "fieldset" }),
  ).toHaveAttribute("inert");
});

it("leaves Escape to a dialog raised over the open menu", async () => {
  const onDialogClose = vi.fn();
  const page = (dialogOpen: boolean) => (
    <LocaleProvider initial="en">
      <Surface />
      <Modal open={dialogOpen} onClose={onDialogClose} labelledBy="edit">
        <Heading size="large" id="edit">
          Edit deal
        </Heading>
      </Modal>
    </LocaleProvider>
  );
  const user = userEvent.setup();
  const { rerender } = rtlRender(page(false));
  await user.click(filterTrigger());
  rerender(page(true));

  await user.keyboard("{Escape}");

  expect(onDialogClose).toHaveBeenCalledOnce();
  expect(filterTrigger()).toHaveAttribute("aria-expanded", "true");
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
