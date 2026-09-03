import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { formatMoney, formatNumber, ordinalNumber } from "../format/format";
import { useLocale } from "../i18n";
import { Button } from "./atoms";
import type { ListChip } from "./listsurface";
import { type ListColumn, ListTable } from "./listtable";

// The list surface every record screen renders into: header, controls, rows and
// footer as one block. The query dials are CONTROLLED and server-backed in the
// product, so each story owns the state a screen would hold and shows what the
// surface does with it. Presentation — which columns are shown, how tight the
// rows are, how wide a column is — is the surface's own business and needs no
// wiring here.

const meta: Meta = {
  title: "Design System/ListTable",
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
  { key: "owner", header: "Owner", cell: (row: Company) => row.owner },
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
  openSortMenu = false,
  ...rest
}: Readonly<{
  rows: Company[];
  /** The page's own name, for the story where the surface IS the page. */
  title?: string;
  /** Defaults to the six this file reads as the reference set. */
  columns?: ListColumn<Company>[];
  pending?: boolean;
  problem?: ReactNode;
  hasMore?: boolean;
  caption?: string;
  note?: string;
  /** Passed through, for the story that holds the page itself. */
  page?: number;
  onPage?: (next: number) => void;
  /**
   * Open the sort menu on mount, for the story that is ABOUT the menu.
   *
   * A press, not a prop on the surface: the menus are the surface's own state
   * by design, so a story that reached inside to set them open would be
   * documenting a shape the product does not have.
   */
  openSortMenu?: boolean;
}>) {
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState("display_name");
  const [chosen, setChosen] = useState<Record<string, string>>({});
  const [view, setView] = useState(0);
  const frame = useRef<HTMLDivElement>(null);

  // Pressed, not set. The menus belong to the surface's own state, so a story
  // that reached in to open one would be documenting a shape the product does
  // not have — and a screenshot of a menu nobody can open is worse than none.
  useEffect(() => {
    if (!openSortMenu) {
      return;
    }
    // Matched on the PREFIX: the dial names the order it is holding
    // ("Sort: Company"), so an equality test against the bare word found
    // nothing and left this story showing a closed menu.
    const trigger = [...(frame.current?.querySelectorAll("button") ?? [])].find(
      (button) => button.textContent?.trim().startsWith("Sort"),
    );
    trigger?.click();
  }, [openSortMenu]);

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
    <div ref={frame}>
      <ListTable<Company>
        rows={shown}
        columns={shownColumns}
        rowKey={(row) => row.id}
        unit="companies"
        action={<Button small>New company</Button>}
        search={{ value: search, onChange: setSearch }}
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
    </div>
  );
}

// Six columns over one page: sortable headers, filter chips behind the Filter
// button, the column picker, the density toggle and the range count.
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
  render: () => <Surface rows={companies(12)} openSortMenu />,
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
          <Button small>Retry</Button>
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

// A caption says what a list IS when it needs saying, and a note says why a
// dial is missing — over a read-only mirror the sort and filter dials are gone
// because the source refuses them.
export const CaptionAndNote: Story = {
  render: () => (
    <Surface
      rows={companies(6)}
      caption="Companies the workspace has captured, newest first."
      note="Sorting and filters read through the source system"
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
            <Button small>Assign owner</Button>
            <Button small onClick={() => setSelected(new Set())}>
              Clear
            </Button>
          </>
        ),
      }}
    />
  );
}

export const Selectable: Story = {
  render: () => <SelectableSurface />,
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
                <Button small variant="ghost">
                  Edit company
                </Button>
                <Button small variant="danger">
                  Archive company
                </Button>
              </div>
            ),
          },
        ]}
      />
    </div>
  ),
};
