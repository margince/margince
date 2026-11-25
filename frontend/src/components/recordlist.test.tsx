/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render as renderNode, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { useT } from "../i18n";
import { emptyPage, type Row, render } from "../testing/listquery.testkit";
import {
  type ListQuery,
  type ListState,
  ListTable,
  useListQuery,
} from "./listquery";
import {
  ABSENT_CELL,
  createdColumn,
  lastActivityCell,
  mineEmptyLine,
  ownerColumn,
  sharedEmptyLine,
  sharedWithMeView,
  standardViews,
} from "./recordlist";

const here = dirname(fileURLToPath(import.meta.url));

// The record lists that share the Owner/Created columns and the standard
// views. A fourth owner-scoped list joins this array, not the copy-paste.
const RECORD_LISTS = [
  "features/contacts/contacts.tsx",
  "features/companies/companies.list.tsx",
  "features/leads/leads.list.tsx",
];

describe("one record list, three record types", () => {
  it("every record list takes Owner and Created from recordlist.tsx, never its own copy", () => {
    // Three lists carried three copies of the Owner column, and a fix on two
    // of them missed the third. A screen that spells its own
    // `key: "owner"` reintroduces exactly that.
    for (const file of RECORD_LISTS) {
      const source = readFileSync(join(here, "..", file), "utf8");
      expect(source, `${file} imports the shared columns`).toMatch(
        /from "[./a-z]*\/recordlist"/,
      );
      expect(source, `${file} defines its own owner column`).not.toMatch(
        /key:\s*"owner"/,
      );
      expect(source, `${file} defines its own created column`).not.toMatch(
        /key:\s*"created"/,
      );
      expect(source, `${file} spells its own Mine view`).not.toMatch(
        /label:\s*"list\.viewMine"/,
      );
    }
  });

  it("the standard views are All, and Mine only for a signed-in reader", () => {
    const t = ((key: string) => key) as Parameters<typeof ownerColumn>[0];
    expect(ownerColumn(t).sort).toBe("owner_id");
    // The zone is what the column RENDERS its dates in; this line asserts what
    // it SORTS on, which the zone cannot change. Any zone the column would
    // accept states the same claim.
    expect(createdColumn(t, "en", "Europe/Berlin").sort).toBe("created_at");
    expect(standardViews(undefined).map((v) => v.label)).toEqual([
      "list.viewAll",
    ]);
    expect(standardViews("u-1").map((v) => v.label)).toEqual([
      "list.viewAll",
      "list.viewMine",
    ]);
    expect(standardViews("u-1")[1]?.filters).toEqual({ owner_id: "u-1" });
    expect(
      standardViews("u-1", { sort: "", mineFirst: true }).map((view) => ({
        label: view.label,
        sort: view.sort,
      })),
    ).toEqual([
      { label: "list.viewMine", sort: "" },
      { label: "list.viewAll", sort: "" },
    ]);
  });
});

describe("an empty date cell", () => {
  afterEach(cleanup);

  // The same mark the other empty cells show, so a blank cell never reads as
  // a value that failed to load.
  it("shows the absent mark for a row with no last activity and no created date", () => {
    const t = ((key: string) => key) as Parameters<typeof createdColumn>[0];
    const row = { last_activity_at: null };
    const created = createdColumn<typeof row>(t, "en", "UTC");
    renderNode(
      <>
        <p data-testid="last">
          {lastActivityCell<typeof row>("en", "UTC")(row)}
        </p>
        <p data-testid="created">{created.cell(row)}</p>
      </>,
    );
    expect(screen.getByTestId("last").textContent).toBe(ABSENT_CELL);
    expect(screen.getByTestId("created").textContent).toBe(ABSENT_CELL);
  });
});

describe("the Shared with me view", () => {
  it("narrows by the reader's grants and names no owner", () => {
    expect(sharedWithMeView().filters).toEqual({ shared_with_me: "true" });
  });
});

describe("the Mine empty note", () => {
  const t = ((key: string) => key) as Parameters<typeof mineEmptyLine>[0]["t"];
  const listOn = (query: Partial<ListQuery>): ListState<{ id: string }> => ({
    rows: [],
    query: {
      q: "",
      sort: "",
      includeArchived: false,
      perPage: 25,
      filters: {},
      ...query,
    },
    setQuery: () => {},
    isPending: false,
    isError: false,
    error: null,
    refetch: () => {},
    hasMore: false,
    loadMore: () => {},
  });
  const note = (query: Partial<ListQuery>) =>
    mineEmptyLine({
      t,
      state: listOn(query),
      viewerId: "u-1",
      unit: "unit.deals",
    });

  it("names ownership when Mine is the only narrowing", () => {
    expect(note({ filters: { owner_id: "u-1" } })).toBeDefined();
  });

  it("leaves a search or another filter to the table's own line", () => {
    expect(note({ q: "acme", filters: { owner_id: "u-1" } })).toBeUndefined();
    expect(
      note({ filters: { owner_id: "u-1", stage_id: "s-1" } }),
    ).toBeUndefined();
  });
});

// Wired the way the contacts screen wires its tabs.
function ContactsList() {
  const t = useT();
  const state = useListQuery<Row>({
    key: "recordlist-empty-views",
    initialSort: "-created_at",
    fetchPage: async () => emptyPage(),
  });
  return (
    <ListTable
      title="Contacts"
      state={state}
      unit="unit.contacts"
      columns={[{ key: "name", header: "Name", cell: (row: Row) => row.name }]}
      rowKey={(row) => row.id}
      views={[...standardViews("u-1"), sharedWithMeView()]}
      emptyLine={
        mineEmptyLine({ t, state, viewerId: "u-1", unit: "unit.contacts" }) ??
        sharedEmptyLine({ t, state, unit: "unit.contacts" })
      }
    />
  );
}

describe("an empty list narrowed by its view alone", () => {
  afterEach(() => {
    cleanup();
    window.location.hash = "";
  });

  it("says only the Shared with me sentence, with Show all as its one way out", async () => {
    window.location.hash = "#/contacts?shared_with_me=true";
    render(<ContactsList />);
    expect(
      await screen.findByText("No contacts shared with you."),
    ).toBeVisible();
    expect(screen.queryByText(/match these filters/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
    expect(screen.getAllByRole("button", { name: "Show all" })).toHaveLength(1);
  });

  it("says only the Mine sentence, with Show all as its one way out", async () => {
    window.location.hash = "#/contacts?owner_id=u-1";
    render(<ContactsList />);
    expect(await screen.findByText("No contacts owned by you.")).toBeVisible();
    expect(screen.queryByText(/match these filters/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
  });

  it("gives a search typed over the view the filters sentence and Clear filters", async () => {
    window.location.hash = "#/contacts?shared_with_me=true&q=zzz";
    render(<ContactsList />);
    expect(
      await screen.findByRole("button", { name: "Clear filters" }),
    ).toBeVisible();
    expect(screen.getByText("No contacts match these filters.")).toBeVisible();
    expect(screen.queryByText("No contacts shared with you.")).toBeNull();
    expect(screen.queryByRole("button", { name: "Show all" })).toBeNull();
  });

  it("reaches Show all by Tab", async () => {
    window.location.hash = "#/contacts?shared_with_me=true";
    render(<ContactsList />);
    const showAll = await screen.findByRole("button", { name: "Show all" });
    const user = userEvent.setup();
    for (
      let press = 0;
      press < 30 && document.activeElement !== showAll;
      press++
    ) {
      await user.tab();
    }
    expect(showAll).toHaveFocus();
  });
});
