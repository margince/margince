/** @vitest-environment happy-dom */

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createPortal } from "react-dom";
import { afterEach, expect, it, vi } from "vitest";
import { TableScroll } from "./atoms";
import {
  DataTable,
  type DataTableColumn,
  type DataTableDetail,
} from "./datatable";
import { Heading } from "./heading";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// A table that scrolls sideways holds columns a pointer can drag to and a
// keyboard cannot reach at all, so the box takes a tab stop and a name — and
// takes neither while it fits, because a tab stop in front of every table in
// the product is a cost every keyboard reader pays for the few that overflow.
// jsdom lays nothing out, so the two widths the decision reads are stubbed on
// the prototype: that is the whole input to it.
function stubBoxWidths(scrollWidth: number, clientWidth: number) {
  for (const [property, value] of [
    ["scrollWidth", scrollWidth],
    ["clientWidth", clientWidth],
  ] as const) {
    vi.spyOn(HTMLDivElement.prototype, property, "get").mockReturnValue(value);
  }
}

const PRODUCT_COLUMNS = [
  { key: "name", header: "Name", render: (row: { name: string }) => row.name },
];
const PRODUCT_ROWS = [{ name: "Consulting Day" }];

it("makes a table's scroll box reachable and named once it overflows", () => {
  stubBoxWidths(930, 654);
  render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  const box = screen.getByRole("region", { name: "Products" });
  expect(box.className).toContain("table-scroll");
  expect(box.getAttribute("tabindex")).toBe("0");
  // One name, the table's, which the region reads rather than repeats.
  expect(box.getAttribute("aria-label")).toBeNull();
  expect(box.getAttribute("aria-labelledby")).toBe(
    screen.getByRole("table").id,
  );
});

it("leaves a table that fits its box out of the tab order", () => {
  stubBoxWidths(654, 654);
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  expect(screen.queryByRole("region")).toBeNull();
  const box = container.querySelector(".table-scroll");
  expect(box?.getAttribute("tabindex")).toBeNull();
});

// `onRowClick` is the whole of what makes a row a link: the handler AND the
// class datatable.css draws the pointer from. A row that opened something
// under a default cursor would look inert until a reader tried it.
it("marks a row as a link only where a click does something", async () => {
  const user = userEvent.setup();
  const opened: string[] = [];
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
      onRowClick={(row) => opened.push(row.name)}
    />,
  );
  await user.click(screen.getByText("Consulting Day"));
  expect(opened).toEqual(["Consulting Day"]);
  expect(container.querySelector("tbody tr")?.className).toBe("rowlink");
});

it("leaves a row it was told does not open without a pointer or a press", async () => {
  const user = userEvent.setup();
  const opened: string[] = [];
  const rows = [{ name: "Consulting Day" }, { name: "Travel" }];
  render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={rows}
      rowKey={(row) => row.name}
      rowTestId={(row) => row.name}
      onRowClick={(row) => opened.push(row.name)}
      rowOpens={(row) => row.name !== "Travel"}
    />,
  );
  expect(screen.getByTestId("Travel").className).toBe("");
  expect(screen.getByTestId("Consulting Day").className).toBe("rowlink");
  await user.click(screen.getByText("Travel"));
  await user.click(screen.getByText("Consulting Day"));
  expect(opened).toEqual(["Consulting Day"]);
});

it("leaves a press on a control in the row, or in a portal it opened, to that control", async () => {
  const user = userEvent.setup();
  const opened: string[] = [];
  const pressed: string[] = [];
  function Portalled() {
    return createPortal(<span>Popover text</span>, document.body);
  }
  render(
    <DataTable
      label="Products"
      columns={[
        ...PRODUCT_COLUMNS,
        {
          key: "verb",
          header: "Verb",
          render: (row: { name: string }) => (
            <>
              <button type="button" onClick={() => pressed.push(row.name)}>
                Price
              </button>
              <Portalled />
            </>
          ),
        },
      ]}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
      rowTestId={(row) => `product-${row.name}`}
      onRowClick={(row) => opened.push(row.name)}
    />,
  );
  await user.click(screen.getByRole("button", { name: "Price" }));
  await user.click(screen.getByText("Popover text"));
  expect(pressed).toEqual(["Consulting Day"]);
  expect(opened).toEqual([]);
  await user.click(screen.getByText("Consulting Day"));
  expect(opened).toEqual(["Consulting Day"]);
  expect(screen.getByTestId("product-Consulting Day").tagName).toBe("TR");
});

it("leaves a row unmarked where nothing answers a click", () => {
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  expect(container.querySelector("tbody tr")?.className).toBe("");
});

it("keeps a table inset unless the caller asks it to bleed", () => {
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  expect(container.querySelector(".table-scroll-bleed")).toBeNull();
});

it("hands bleed to the scroll box that holds the table", () => {
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
      bleed
    />,
  );
  const box = container.querySelector(".table-scroll");
  expect(box?.classList.contains("table-scroll-bleed")).toBe(true);
  expect(box?.firstElementChild?.classList.contains("table")).toBe(true);
});

it("names an overflowing box by a heading already on the page, without a copy", () => {
  stubBoxWidths(930, 654);
  render(
    <>
      <Heading size="small" as="h3" id="grants-head">
        Grants
      </Heading>
      <TableScroll label={{ labelledBy: "grants-head" }}>
        <table className="table" aria-labelledby="grants-head">
          <tbody>
            <tr>
              <td>Admin</td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </>,
  );
  const box = screen.getByRole("region", { name: "Grants" });
  expect(box.getAttribute("aria-labelledby")).toBe("grants-head");
  expect(box.hasAttribute("aria-label")).toBe(false);
});

it("lets a hand-drawn table bleed through the same box", () => {
  const { container } = render(
    <TableScroll label="Spend by task" bleed>
      <table className="table">
        <tbody>
          <tr>
            <td>Drafting</td>
          </tr>
        </tbody>
      </table>
    </TableScroll>,
  );
  expect(
    container.querySelector(".table-scroll-bleed > .table"),
  ).not.toBeNull();
});

// The first and the last cell of every row, header included, carry the pane's
// padding, so the first column's text stands at a PanelBody's x.
it("pads a bleeding table's outer cells to the pane", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const css = readFileSync(join(here, "atoms.css"), "utf8");
  for (const [edge, side] of [
    ["first-child", "start"],
    ["last-child", "end"],
  ]) {
    expect(css).toContain(
      `.panel > .table-scroll-bleed > .table > * > tr > :${edge} {\n  padding-inline-${side}: var(--padPanel);`,
    );
  }
});

type DemoMember = { name: string; email: string; role: string; team: string };

const MEMBER = {
  name: "Ana Weber",
  email: "ana@example.com",
  role: "Admin",
  team: "",
};

const MEMBER_COLUMNS: DataTableColumn<DemoMember>[] = [
  { key: "name", header: "Name", render: (row: DemoMember) => row.name },
  { key: "email", header: "Email", render: (row: DemoMember) => row.email },
  { key: "team", header: "Team", render: (row: DemoMember) => row.team },
  {
    key: "role",
    header: "Role",
    fold: "end",
    render: (row: DemoMember) => row.role,
  },
];

function renderMembers(fold: boolean, columns = MEMBER_COLUMNS) {
  return render(
    <DataTable
      label="Members"
      columns={columns}
      rows={[MEMBER]}
      rowKey={(row) => row.name}
      fold={fold}
    />,
  );
}

function foldPlaces(container: HTMLElement) {
  return [...container.querySelectorAll("tbody td")].map((cell) =>
    cell.getAttribute("data-fold"),
  );
}

it("leaves a table that does not fold to its native roles and places", () => {
  const { container } = renderMembers(false);
  expect(container.querySelector(".table-scroll-fold")).toBeNull();
  expect(container.querySelector("[role]:not(.table-scroll)")).toBeNull();
  expect(foldPlaces(container)).toEqual([null, null, null, null]);
});

// datatable.css keys line one and line two on these places alone.
it("places each cell of a folding table on its line", () => {
  const { container } = renderMembers(true);
  expect(container.querySelector(".table-scroll-fold > .table")).not.toBeNull();
  expect(foldPlaces(container)).toEqual(["title", "rest", "rest", "end"]);
});

it("takes the title from the column that names it, and hides on request", () => {
  const { container } = renderMembers(true, [
    { ...MEMBER_COLUMNS[0], fold: "hide" },
    { ...MEMBER_COLUMNS[1], fold: "title" },
    MEMBER_COLUMNS[2],
    MEMBER_COLUMNS[3],
  ]);
  expect(foldPlaces(container)).toEqual(["hide", "title", "rest", "end"]);
});

it("titles a fold by its first unplaced column when none names one", () => {
  const { container } = renderMembers(true, [
    MEMBER_COLUMNS[3],
    ...MEMBER_COLUMNS.slice(0, 3),
  ]);
  expect(foldPlaces(container)).toEqual(["end", "title", "rest", "rest"]);
});

// A folded row is laid out as flex, which strips a native row's role in
// Safari, so the roles are spelled. Every cell keeps a heading at its column,
// an empty one included, so "Role, Admin" is what a reader hears.
it("keeps every folded cell under the heading that names it", () => {
  renderMembers(true);
  const headers = screen
    .getAllByRole("columnheader")
    .map((header) => header.textContent);
  expect(headers).toEqual(["Name", "Email", "Team", "Role"]);
  const [, row] = screen.getAllByRole("row");
  const cells = [...row.querySelectorAll("[role='cell']")];
  expect(cells.map((cell) => cell.textContent)).toEqual([
    "Ana Weber",
    "ana@example.com",
    "",
    "Admin",
  ]);
  expect(headers[cells.findIndex((cell) => cell.textContent === "Admin")]).toBe(
    "Role",
  );
  expect(screen.getByRole("table").getAttribute("role")).toBe("table");
});

it("reads a hidden heading to assistive tech and draws none of it", () => {
  renderMembers(false, [
    ...MEMBER_COLUMNS.slice(0, 3),
    { ...MEMBER_COLUMNS[3], headerHidden: true },
  ]);
  const heading = screen.getByRole("columnheader", { name: "Role" });
  expect(heading.firstElementChild?.className).toBe("sr-only");
  expect(heading.firstElementChild?.textContent).toBe("Role");
  expect(
    screen.getByRole("columnheader", { name: "Name" }).children,
  ).toHaveLength(0);
});

const datatableCss = () =>
  readFileSync(
    join(dirname(fileURLToPath(import.meta.url)), "datatable.css"),
    "utf8",
  );

// display: none would drop the heading and an empty cell from the
// accessibility tree, and every later cell would be read under the wrong name.
it("hides a folded heading and an empty cell by clipping, never by display", () => {
  const css = datatableCss();
  const fold = css.slice(css.indexOf("@container table-fold"));
  expect(fold).toContain(".table-scroll-fold > .table > thead,");
  expect(fold).toContain(':is(:empty, [data-fold="hide"])');
  expect(fold).toContain("clip-path: inset(50%);");
  // `folded` swaps a figure for its phrase, and only one of the two may be heard.
  const swap =
    /\.table-scroll-fold \.datatable-unfolded \{\s*display: none;\s*\}/;
  expect(fold).toMatch(swap);
  expect(fold.replace(swap, "")).not.toContain("display: none");
});

// A glyph between caption cells is left at a line's end, or leads the next
// line, wherever a narrow row wraps; space alone cannot strand.
it("separates a folded caption's cells by space, never by a glyph", () => {
  const css = datatableCss();
  const fold = css.slice(css.indexOf("@container table-fold"));
  expect(fold).not.toMatch(/content:\s*"[^"]/);
  expect(fold).toContain("column-gap: var(--space-3);");
});

it("folds on the table's own width rather than the viewport's", () => {
  const css = datatableCss();
  expect(css).toContain("container: table-fold / inline-size;");
  expect(css).toContain("@container table-fold (width < 36rem)");
  expect(css).not.toMatch(/@media[^{]*width/);
});

it("pins the first column of a table that asks, on DataTable and by hand", () => {
  const { container } = render(
    <>
      <DataTable
        label="Products"
        columns={PRODUCT_COLUMNS}
        rows={PRODUCT_ROWS}
        rowKey={(row) => row.name}
        stickyFirst
      />
      <TableScroll label="Grants" stickyFirst>
        <table className="grant-matrix">
          <tbody>
            <tr>
              <td>contacts.read</td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </>,
  );
  const pinned = container.querySelectorAll(".table-scroll-sticky > table");
  expect(pinned).toHaveLength(2);
});

// The pinned cell slides over the rest, so the pane's see-through ground is
// laid over the page beneath it.
it("gives a pinned cell an opaque ground at rest and under the pointer", () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const css = readFileSync(join(here, "atoms.css"), "utf8");
  expect(css).toContain(
    ".panel .table-scroll-sticky > table > * > tr > :first-child {\n  background: linear-gradient(var(--pane), var(--pane)), var(--bgPage);",
  );
  expect(css).toContain(
    ".table-scroll-sticky > .table > tbody > tr:hover > :first-child {\n  background: var(--bgHover);",
  );
});

const CALLS = [
  { id: "a", task: "Triage", trace: "Two attempts" },
  { id: "b", task: "Enrich", trace: "One attempt" },
];
type Call = (typeof CALLS)[number];
const CALL_COLUMNS: DataTableColumn<Call>[] = [
  { key: "task", header: "Task", render: (call) => call.task },
];

const TRACE: DataTableDetail<Call> = {
  header: "Detail",
  toggleLabel: (call) => `Show ${call.task}`,
  render: (call) => <p>{call.trace}</p>,
};

function CallLog({
  fold,
  detail = TRACE,
}: Readonly<{ fold?: boolean; detail?: DataTableDetail<Call> }>) {
  return (
    <DataTable
      label="Calls"
      fold={fold}
      columns={CALL_COLUMNS}
      rows={CALLS}
      rowKey={(call) => call.id}
      rowTestId={(call) => `call-${call.id}`}
      detail={detail}
    />
  );
}

it("opens a row's detail under it, from its toggle or a press anywhere on the row", async () => {
  const user = userEvent.setup();
  render(<CallLog />);
  const toggle = screen.getByRole("button", { name: "Show Triage" });
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  const region = document.getElementById(
    toggle.getAttribute("aria-controls") ?? "",
  );
  expect(region?.tagName).toBe("TD");
  expect(region?.getAttribute("colspan")).toBe("2");
  expect(region?.closest("tr")?.hidden).toBe(true);

  await user.click(screen.getByText("Triage"));
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  expect(region?.textContent).toBe("Two attempts");
  expect(screen.getByTestId("call-a").className).toContain("datatable-open");
  expect(screen.queryByText("One attempt")).toBeNull();

  await user.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(region?.textContent).toBe("");
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
});

it("leaves which rows are open to a caller that holds them", async () => {
  const user = userEvent.setup();
  const toggled: string[] = [];
  render(
    <CallLog
      detail={{
        ...TRACE,
        expanded: new Set(["b"]),
        onToggle: (key) => toggled.push(key),
      }}
    />,
  );
  expect(screen.getByText("One attempt")).toBeTruthy();
  await user.click(screen.getByText("Triage"));
  expect(toggled).toEqual(["a"]);
  expect(screen.queryByText("Two attempts")).toBeNull();
});

it("gives a row with nothing to open no toggle and no pointer", async () => {
  const user = userEvent.setup();
  render(<CallLog detail={{ ...TRACE, has: (call) => call.id === "a" }} />);
  expect(screen.queryByRole("button", { name: "Show Enrich" })).toBeNull();
  expect(screen.getByTestId("call-b").className).toBe("");
  expect(screen.getByTestId("call-a").className).toBe("rowlink");
  await user.click(screen.getByText("Enrich"));
  expect(screen.queryByText("One attempt")).toBeNull();
});

it("points the toggle at the caller's element when nothing opens under the row", () => {
  render(
    <CallLog
      detail={{
        header: "Detail",
        toggleLabel: TRACE.toggleLabel,
        controls: (call) => `trace-${call.id}`,
      }}
    />,
  );
  const toggle = screen.getByRole("button", { name: "Show Triage" });
  expect(toggle.getAttribute("aria-controls")).toBe("trace-a");
  expect(document.querySelector(".datatable-detail")).toBeNull();
});

it("keeps a folded row's toggle at the end of its first line", () => {
  render(<CallLog fold />);
  const toggle = screen.getByRole("button", { name: "Show Triage" });
  expect(toggle.closest("td")?.getAttribute("data-fold")).toBe("end");
  expect(
    screen
      .getByRole("columnheader", { name: "Detail" })
      .querySelector(".sr-only"),
  ).not.toBeNull();
});

it("names the table itself, whether or not its box scrolls", () => {
  stubBoxWidths(654, 654);
  render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  expect(screen.getByRole("table", { name: "Products" })).toBeTruthy();
});

it("leaves a press on any control that keeps its own state to that control", async () => {
  const user = userEvent.setup();
  const opened: string[] = [];
  const controls = [
    <summary key="summary">Summary</summary>,
    ...["switch", "menuitem", "checkbox", "tab"].map((controlRole) => (
      <span key={controlRole} role={controlRole} tabIndex={0}>
        {controlRole}
      </span>
    )),
  ];
  render(
    <DataTable
      label="Products"
      columns={[
        ...PRODUCT_COLUMNS,
        { key: "controls", header: "Controls", render: () => controls },
      ]}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
      onRowClick={(row) => opened.push(row.name)}
    />,
  );
  for (const name of ["Summary", "switch", "menuitem", "checkbox", "tab"]) {
    await user.click(screen.getByText(name));
  }
  expect(opened).toEqual([]);
});
