import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  formatDateAbbrev,
  formatMoney,
  formatNumber,
  ordinalNumber,
} from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale } from "../i18n";
import { Badge, Button, OverflowMenu } from "./atoms";
import { Avatar } from "./avatar";
import type { ListChip } from "./listsurface";
import { CellStrip, type ListColumn, ListTable } from "./listtable";
import { SelectionBar } from "./selectionbar";

// The list surface every record screen renders into: header, controls, rows and
// footer as one block. The query dials are CONTROLLED and server-backed in the
// product, so each story owns the state a screen would hold and shows what the
// surface does with it. Presentation — which columns are shown, how tight the
// rows are, how wide a column is — is the surface's own business and needs no
// wiring here.

const meta: Meta = {
  title: "Components/Text and data display/List table",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Company = {
  id: string;
  name: string;
  industry: string;
  size: string;
  owner: string;
  region: string;
  valueMinor: number;
  createdAt: string;
};

const INDUSTRIES = ["Manufacturing", "Logistics", "Healthcare", "SaaS"];
const REGIONS = ["DACH", "Nordics", "Benelux"];

function companies(count: number): Company[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `c${index + 1}`,
    name: `Company ${String(index + 1).padStart(2, "0")}`,
    industry: INDUSTRIES[index % INDUSTRIES.length] ?? "SaaS",
    size: index % 3 === 0 ? "201-500" : "11-50",
    owner: index % 2 === 0 ? "Lars" : "Mia",
    region: REGIONS[index % REGIONS.length] ?? "DACH",
    valueMinor: (index + 1) * 12_500,
    createdAt: new Date(Date.UTC(2026, 0, 1 + index * 9)).toISOString(),
  }));
}

const columns = [
  {
    key: "name",
    header: "Company",
    // fixed keeps it out of the column picker and pins it left once the table
    // scrolls sideways.
    fixed: true,
    sort: "display_name",
    cell: (row: Company) => <strong>{row.name}</strong>,
  },
  { key: "industry", header: "Industry", cell: (row: Company) => row.industry },
  { key: "size", header: "Size", cell: (row: Company) => row.size },
  {
    key: "owner",
    header: "Owner",
    // A colleague's face before their name, and a colleague column's width: no
    // wider than a name needs, however wide the page.
    holds: "colleague" as const,
    cell: (row: Company) => (
      <span className="avatar-row">
        <Avatar identity={row.owner} name={row.owner} />
        <span>{row.owner}</span>
      </span>
    ),
  },
  { key: "region", header: "Region", cell: (row: Company) => row.region },
  {
    key: "value",
    header: "Pipeline",
    numeric: true,
    sort: "amount_minor",
    // Through the repo's money formatter, like a real screen: this file is the
    // reference copy of the surface, so a hand-rolled currency string here is
    // the version somebody else copies.
    cell: (row: Company) => formatMoney(row.valueMinor, "EUR", "en"),
  },
  {
    key: "created",
    header: "Created",
    holds: "date" as const,
    cell: (row: Company) => formatDateAbbrev(row.createdAt, "en", viewerZone()),
  },
];

const chips: readonly ListChip[] = [
  {
    key: "industry",
    label: "Industry",
    allLabel: "All industries",
    options: INDUSTRIES.map((value) => ({ value, label: value })),
  },
  {
    key: "owner",
    label: "Owner",
    allLabel: "Any owner",
    options: [
      { value: "Lars", label: "Lars" },
      { value: "Mia", label: "Mia" },
    ],
  },
];

/**
 * A screen's worth of state around the surface: the search term, the sort
 * string the server would receive, the chosen filters and the view. Filtering
 * happens here only so the stories show real rows change — in the product the
 * server answers all four.
 */
function Surface({
  rows,
  columns: shownColumns = columns,
  noAction = false,
  ...rest
}: Readonly<{
  rows: Company[];
  /** The page's own name, for the story where the surface IS the page. */
  title?: string;
  /** Defaults to the seven this file reads as the reference set. */
  columns?: ListColumn<Company>[];
  pending?: boolean;
  problem?: ReactNode;
  hasMore?: boolean;
  hideShortChrome?: boolean;
  /** Leaves the header verb out, for the stories of a list inside a settings card. */
  noAction?: boolean;
  caption?: string;
  note?: string;
  /** The caller's sentence under the empty line, for the story that shows it. */
  emptyNote?: string;
  /** Passed through, for the story that holds the page itself. */
  page?: number;
  onPage?: (next: number) => void;
}>) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("display_name");
  const [chosen, setChosen] = useState<Record<string, string>>({});
  const [view, setView] = useState(0);

  const needle = search.trim().toLowerCase();
  // Only the two attributes the chips offer, read by name rather than by
  // indexing the row with a widened key.
  const attribute = (row: Company, key: string) =>
    key === "industry" ? row.industry : key === "owner" ? row.owner : "";
  const shown = rows.filter(
    (row) =>
      (!needle || row.name.toLowerCase().includes(needle)) &&
      Object.entries(chosen).every(
        ([key, value]) => !value || attribute(row, key) === value,
      ) &&
      (view === 0 || row.region === "DACH"),
  );

  return (
    <ListTable<Company>
      rows={shown}
      columns={shownColumns}
      rowKey={(row) => row.id}
      unit="companies"
      action={noAction ? undefined : <Button>New company</Button>}
      search={{ value: search, answered: search, onChange: setSearch }}
      sort={{ value: sort, onChange: setSort }}
      chips={chips}
      chosen={chosen}
      onChipChange={(key, value) =>
        setChosen((prev) => ({ ...prev, [key]: value }))
      }
      archived={{ checked: false, onChange: () => undefined }}
      views={[{ label: "All" }, { label: "DACH" }]}
      activeView={view}
      onViewChange={setView}
      {...rest}
    />
  );
}

/**
 * Opens one of the toolbar's menus the way a reader does — by pressing it.
 *
 * A press and not a prop: the menus are the surface's own state by design, so
 * a story that reached in to set one open would document a shape the product
 * does not have.
 *
 * The trigger arrives as a pattern rather than a word because two of them are
 * only findable that way: the sort dial names the order it is holding
 * ("Sort: Company"), and every sortable column header is a button called
 * "Sort by <column>" that a bare prefix would match as well.
 */
function openMenu(trigger: RegExp) {
  return async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const page = within(canvasElement);
    await userEvent.click(await page.findByRole("button", { name: trigger }));
  };
}

// Six columns over one page: sortable headers, filter chips behind the Filter
// button, the Display menu holding the columns and the density, and the range
// count.
export const Default: Story = {
  render: () => <Surface rows={companies(12)} />,
};

// More rows than a page holds, so the footer's pager has pages to walk. The
// count reads as a range, and the page resets whenever the set narrows.
export const Paged: Story = {
  render: () => <Surface rows={companies(60)} />,
};

// The sort dial as a menu, which is the route a column header cannot be: it
// offers every orderable column including the ones the reader has hidden or a
// phone has no room for, it says which way the current one runs, and it offers
// the server's own order — a state a saved view can ask for and a header
// cannot. Opened here with a sort already applied, so the tick and the
// direction arrow are both on screen.
export const SortedByAMenu: Story = {
  render: () => <Surface rows={companies(12)} />,
  play: openMenu(/^Sort(:|$)/),
};

// The Display menu: how the grid is DRAWN, under one trigger. Density and the
// optional columns are one question to a reader — both answer "show me more of
// this at once" — and each group is named a rung under the menu's own head, so
// the panel reads as one question in two parts rather than as three headings.
export const DisplayMenuOpen: Story = {
  render: () => <Surface rows={companies(12)} />,
  play: openMenu(/^Display$/),
};

// The filter menu's SECOND step, which is the half a list of attributes cannot
// show: the attribute's own values, with the head standing as the way back to
// the list they were chosen from.
export const FilterAttributeStep: Story = {
  render: () => <Surface rows={companies(12)} />,
  play: async (context) => {
    await openMenu(/^Filter$/)(context);
    const page = within(context.canvasElement);
    await userEvent.click(
      await page.findByRole("button", { name: "Industry" }),
    );
    // The head is the way back once a level has been stepped into, and waiting
    // on it is what makes the story's own subject the thing it settles for.
    await page.findByRole("button", { name: /Industry Back to all filters/ });
  },
};

// The same pager, with the PAGE held by the caller. A list screen keeps it in
// the address, so a reader who paged through a list and opened a record comes
// back to the page they left; here the holder is a story so the contract is
// visible on its own — the number in the caption is the caller's, and the strip
// is drawing what it was handed rather than what it counted.
//
// Narrowing still resets it: type in the search box and the caller is told 1.
// That reset is compared by VALUE, not by counting effect runs, because an
// effect runs on arrival — twice under StrictMode, and again for any dial that
// settles a tick after mount — and a reset that believed those took the reader
// off the page their own address had asked for.
export const PagedByTheCaller: Story = {
  render: () => {
    function Holder() {
      const [page, setPage] = useState(3);
      return (
        <div style={{ display: "grid", gap: "var(--space-2)" }}>
          <Surface
            rows={companies(60)}
            page={page}
            onPage={setPage}
            // A page number is a POSITION, so it stays bare in every locale:
            // grouped, page 1204 reads "1.204" and stops matching the page a
            // reader types into the box beside it.
            caption={`The caller is holding page ${ordinalNumber(page)}`}
          />
        </div>
      );
    }
    return <Holder />;
  },
};

// hasMore is what a keyset cursor reports: no total, so the pager numbers the
// pages in hand and Next stays enabled on the last of them to fetch the next
// rather than the strip inventing a page count. Six pages in hand is enough to
// show the whole strip: page one anchored, the window around the current page,
// and a gap standing for the pages between them.
export const MoreToFetch: Story = {
  render: () => <Surface rows={companies(150)} hasMore />,
};

// The first page is in flight. The header, the dials and the primary action
// belong to the screen rather than to the response, so they stay put and only
// the body reports that rows are coming.
export const Pending: Story = {
  render: () => <Surface rows={[]} pending />,
};

// The read failed. Same rule as pending: the surface stays, the body carries
// the reason and whatever retry the caller supplies.
export const Failed: Story = {
  render: () => (
    <Surface
      rows={[]}
      problem={
        <>
          <p>Couldn't load this view.</p>
          <Button>Retry</Button>
        </>
      }
    />
  ),
};

// Nothing exists yet, which is not the same as nothing matching: this copy does
// not offer to clear filters, because there are none to clear. Search or filter
// the Default story down to nothing to see the other empty state.
export const Empty: Story = {
  render: () => <Surface rows={[]} />,
};

// The caller's own note under that line, for an emptiness only the screen can
// explain. It sits a step below the generic sentence, close enough to read as
// the same answer rather than an unrelated second fact.
export const EmptyWithANote: Story = {
  render: () => (
    <Surface
      rows={[]}
      emptyNote="Companies arrive from the nightly import — the first run lands tomorrow."
    />
  ),
};

// A caption says what a list IS when it needs saying.
export const Caption: Story = {
  render: () => (
    <Surface
      rows={companies(6)}
      caption="Companies the workspace has captured, newest first."
    />
  ),
};

// The surface AS THE PAGE: it prints the page's own name in the header, on the
// line that already carries the view tabs and the count. This is the arrangement
// every record list uses — the shell prints no heading above a screen that
// passes `title` — and what to read for is the head at a narrow width, where the
// name takes the first line and the tabs continue under it rather than both
// being squeezed.
export const AsThePage: Story = {
  render: () => <Surface rows={companies(12)} title="Companies" />,
};

// Narrow enough that six columns do not fit. The identity column stays pinned
// to the left edge and casts one continuous shade over the columns sliding
// under it; drag a header's trailing edge to resize a column, which widens the
// table rather than squeezing its neighbours. Under 720px the rows become
// cards instead — resize the preview to see that.
export const PinnedWhileScrolling: Story = {
  render: () => (
    <div style={{ maxWidth: 720 }}>
      <Surface rows={companies(8)} />
    </div>
  ),
};

// Row selection for a bulk action: the checkbox sits in the identity cell —
// the one that stays put while the rest scrolls — and the bar above the grid
// carries the count and the verbs. Two rows come pre-selected so the bar is
// on screen; the screen owns what the verbs do.
function SelectableSurface() {
  const { locale } = useLocale();
  const rows = companies(8);
  const [selected, setSelected] = useState<ReadonlySet<string>>(
    new Set(["c2", "c5"]),
  );
  const toggle = (row: Company) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(row.id)) {
        next.delete(row.id);
      } else {
        next.add(row.id);
      }
      return next;
    });
  return (
    <ListTable<Company>
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="companies"
      selection={{
        selected,
        onToggle: toggle,
        label: (row) => `Select ${row.name}`,
        bar: (
          <>
            <span className="t-caption">
              {formatNumber(selected.size, locale)} selected
            </span>
            <Button>Assign owner</Button>
            <Button onClick={() => setSelected(new Set())}>Clear</Button>
          </>
        ),
      }}
    />
  );
}

export const Selectable: Story = {
  render: () => <SelectableSurface />,
};

// The same bar over a list that is not a table: the count and the verbs, in
// the box the table draws over its grid.
export const SelectionBarAlone: Story = {
  render: () => (
    <SelectionBar>
      <span className="t-caption">3 selected</span>
      <Button>Clear selection</Button>
      <Button variant="primary">Mark done</Button>
    </SelectionBar>
  ),
};

// The state the settings tree put this surface in, and the one it read as
// broken: a 720px reading column, five columns, and the last of them a pair of
// labelled buttons. Every column sits at the floor its kind declares and the
// BODY scrolls sideways for the rest — nothing is crushed, nothing is
// half-drawn, and the page behind it does not move. The box takes a tab stop
// and announces itself as "products" while it is holding something past its
// right edge; scroll it back and the tab stop goes away again.
//
// The width is set here rather than by a viewport global on purpose: this is
// not a phone (where the rows become cards), it is a DESKTOP window with a
// narrow reading column, which is the case no other story in this file draws.
export const InASettingsColumn: Story = {
  name: "In a settings column (720px)",
  render: () => (
    <div style={{ maxWidth: "720px" }}>
      <Surface
        rows={companies(3)}
        columns={[
          ...columns,
          {
            key: "actions",
            header: "Actions",
            // A verbs column: sized by the buttons, in pixels, rather than by
            // a share of a column this narrow.
            verbs: true,
            cell: () => (
              <div style={{ display: "flex", gap: "var(--space-2)" }}>
                <Button variant="ghost">Edit company</Button>
                <Button variant="danger">Archive company</Button>
              </div>
            ),
          },
        ]}
      />
    </div>
  ),
};

// Several pills in ONE cell, and what happens when they do not fit. The cells
// are the ones `CellStrip` serves: a relationship column carrying three badges
// at once, a score badge with the reason beside it, and a deal's age with its
// stalled flag. None fits the share its column gets at this width, and none
// takes a second line — a cell that wrapped would push every row below it down.
// The reason ends in an ellipsis while its badge stays whole, and the three
// badges each ellipsise their own label; nothing is cut mid-word at the column
// edge. The reader's answer is to widen the column (drag a header's trailing
// edge) and nobody else's row moves.
export const CellStrips: Story = {
  name: "Cell strips",
  render: () => (
    <div style={{ maxWidth: "720px" }}>
      <Surface
        rows={companies(4)}
        columns={[
          {
            key: "name",
            header: "Company",
            fixed: true,
            cell: (row: Company) => <strong>{row.name}</strong>,
          },
          {
            key: "relationship",
            header: "Relationship",
            // Multi-valued: an account can be a partner AND a customer, and a
            // column showing only the first would make the second look untrue.
            cell: () => (
              <CellStrip>
                {["Customer", "Partner", "Supplier"].map((kind) => (
                  <Badge key={kind}>{kind}</Badge>
                ))}
              </CellStrip>
            ),
          },
          {
            key: "score",
            header: "Score",
            cell: (row: Company) => {
              const warm = row.owner === "Lars";
              return (
                <CellStrip>
                  <Badge tone={warm ? "success" : "warning"}>
                    {warm ? "Warm" : "Cooling"}
                  </Badge>
                  <span className="t-caption">
                    {warm
                      ? "Replied inside a day to the renewal proposal"
                      : "No qualifying signal for three weeks"}
                  </span>
                </CellStrip>
              );
            },
          },
          {
            key: "signal",
            header: "Last signal",
            numeric: true,
            cell: (row: Company) => (
              <CellStrip>
                <span>{row.owner === "Lars" ? "2 days" : "3 weeks"}</span>
                {row.owner !== "Lars" && <Badge tone="warning">Stalled</Badge>}
              </CellStrip>
            ),
          },
        ]}
      />
    </div>
  ),
};

// A list that fits the smallest page size has no pages to walk. Nothing narrows
// it, so it has nothing to search either: no count, no dials, no pager.
export const ShortListWithoutChrome: Story = {
  render: () => <Surface rows={companies(12)} hideShortChrome noAction />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Sort by Company" });
    await expect(
      canvas.queryByRole("combobox", { name: "Rows per page" }),
    ).toBeNull();
    await expect(canvas.queryByRole("searchbox")).toBeNull();
    await expect(canvas.queryByText(/sorted by/)).toBeNull();
  },
};

// The header verb outlives the count it stood beside.
export const ShortListKeepsVerb: Story = {
  render: () => <Surface rows={companies(3)} hideShortChrome />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Sort by Company" });
    // Under the narrow width the verbs fold into one menu.
    const more = canvas.queryByRole("button", { name: "More actions" });
    if (more) await userEvent.click(more);
    await expect(
      await within(canvasElement.ownerDocument.body).findByRole("button", {
        name: "New company",
      }),
    ).toBeVisible();
    await expect(canvas.queryByText(/sorted by/)).toBeNull();
  },
};

// No rows reads as one line, not as a table of headings over nothing. The
// archived switch stays, since archived rows may be all the list holds.
export const ShortListEmpty: Story = {
  render: () => <Surface rows={[]} hideShortChrome noAction />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("No companies yet.")).toBeVisible();
    await expect(canvas.queryByRole("columnheader")).toBeNull();
    await expect(
      canvas.getByRole("checkbox", { name: "Show archived" }),
    ).toBeVisible();
    await expect(canvas.queryByRole("searchbox")).toBeNull();
  },
};

export const ShortListEmptyOnPhone: Story = {
  ...ShortListEmpty,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// Past the smallest page size the pager and the dials stay. A larger size may
// fit every row, and the reader can still take that size back.
export const LongerListKeepsPager: Story = {
  render: () => <Surface rows={companies(40)} hideShortChrome />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("navigation", { name: "Pages" });
    await expect(
      canvas.getByRole("combobox", { name: "Rows per page" }),
    ).toBeVisible();
    await expect(canvas.getByRole("searchbox")).toBeVisible();
  },
};

const menuColumns: ListColumn<Company>[] = [
  ...columns.slice(0, 3),
  {
    key: "menu",
    header: "Actions",
    verbs: "menu",
    cell: (row: Company) => (
      <OverflowMenu label={`Actions for ${row.name}`}>
        <Button>Edit</Button>
        <Button variant="danger">Archive</Button>
      </OverflowMenu>
    ),
  },
];

async function openRowMenu({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Actions for Company 01" }),
  );
  await within(document.body).findByRole("button", { name: "Edit" });
}

// Several verbs per row fold into one trailing menu. Its heading is read and
// never drawn, so forced colours show nothing over the column either.
export const RowMenu: Story = {
  render: () => <Surface rows={companies(4)} columns={menuColumns} />,
  play: openRowMenu,
};

// On a phone each row is a card, and the menu keeps its corner of it.
export const RowMenuOnPhone: Story = {
  render: () => <Surface rows={companies(4)} columns={menuColumns} />,
  play: openRowMenu,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const cardColumns: ListColumn<Company>[] = [
  ...columns.slice(0, 2),
  {
    key: "email",
    header: "E-mail",
    cell: (row: Company) => (
      <a href={`mailto:${row.id}`}>
        {`procurement.department.${row.id}@a-very-long-company-domain.example`}
      </a>
    ),
  },
  // A renderer that wraps nothing in an element, as the Last activity cell does.
  { key: "seen", header: "Last activity", cell: () => <span /> },
];

// On a phone a card keeps a long value inside its edge, and a field with
// nothing in it shows no label.
export const CardsOnPhone: Story = {
  render: () => <Surface rows={companies(3)} columns={cardColumns} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const value = await canvas.findByText(/^procurement\.department\.c1@/);
    const card = value.closest("tr");
    await expect(value.getBoundingClientRect().right).toBeLessThanOrEqual(
      card?.getBoundingClientRect().right ?? 0,
    );
    for (const cell of canvasElement.querySelectorAll(
      'td[data-label="Last activity"]',
    )) {
      await expect(cell).not.toBeVisible();
    }
  },
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const longHeaderColumns: ListColumn<Company>[] = [
  ...columns.slice(0, 4),
  {
    key: "region",
    header: "Relationship to the account",
    cell: (row: Company) => row.region,
  },
  {
    key: "value",
    header: "Open deals this quarter",
    numeric: true,
    sort: "amount_minor",
    cell: (row: Company) => formatNumber(row.valueMinor / 12_500, "en"),
  },
  {
    key: "renewal",
    header: "Vertragsverlängerungskündigungsfrist",
    cell: (row: Company) => row.size,
  },
];

// A header longer than its column wraps, and the column keeps its width.
// Once the column is sorted, its arrow stays clear of the resize grip.
export const LongHeadersWrap: Story = {
  render: () => (
    <div style={{ maxWidth: "62rem" }}>
      <Surface rows={companies(4)} columns={longHeaderColumns} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Sort by Open deals this quarter",
      }),
    );
    for (const header of canvasElement.querySelectorAll("thead th")) {
      await expect(header.scrollWidth).toBeLessThanOrEqual(header.clientWidth);
    }
    const arrow = canvasElement.querySelector(".lt-sort.on .lt-arrow");
    const grip = arrow?.closest("th")?.querySelector(".lt-grip");
    if (!arrow || !grip)
      throw new Error("the sorted header drew no arrow or grip");
    await expect(arrow.getBoundingClientRect().right).toBeLessThanOrEqual(
      grip.getBoundingClientRect().left,
    );
  },
};

const WIDE_FIGURES = ["₫120,000,000,000", "₫4,500,000", "₫900,000"];

// Figures wider than a numeric column's share, in a box too narrow for the
// row. The column takes its widest figure, so no amount reads as another.
export const WideFigures: Story = {
  name: "Wide figures",
  render: () => (
    <div style={{ maxWidth: "420px" }}>
      <Surface
        rows={companies(3)}
        columns={[
          {
            key: "name",
            header: "Company",
            fixed: true,
            cell: (row: Company) => <strong>{row.name}</strong>,
          },
          {
            key: "value",
            header: "Value",
            numeric: true,
            cell: (row: Company) => (
              <span className="t-num">
                {WIDE_FIGURES[Number(row.id.slice(1)) - 1]}
              </span>
            ),
          },
          {
            key: "region",
            header: "Region",
            cell: (row: Company) => row.region,
          },
        ]}
      />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const figure = await within(canvasElement).findByRole("cell", {
      name: WIDE_FIGURES[0],
    });
    const ink = document.createRange();
    ink.selectNodeContents(figure);
    const style = getComputedStyle(figure);
    await waitFor(() => {
      const cell = figure.getBoundingClientRect();
      const text = ink.getBoundingClientRect();
      expect(text.left).toBeGreaterThanOrEqual(
        cell.left + Number.parseFloat(style.paddingLeft) - 0.5,
      );
      expect(text.right).toBeLessThanOrEqual(
        cell.right - Number.parseFloat(style.paddingRight) + 0.5,
      );
    });
  },
};
