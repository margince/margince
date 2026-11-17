/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { FOLD_MAX_WIDTH } from "../lib/viewport";
import { openView, openViewStep } from "../testing/listview.testkit";
import { phoneRow } from "../testing/phonerow";
import { stubWindowWidth } from "../testing/phonerow.testkit";
import type { ListColumn } from "./listtable";
import { ListTable } from "./listtable";
import { render } from "./listtable.testkit";
import { ListViewMenu, type ViewRow } from "./listviewmenu";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Deal = {
  id: string;
  name: string;
  stage: string;
  amount: string;
  company: string | null;
  owner: string | null;
  source: string;
};

const deal: Deal = {
  id: "d1",
  name: "Brandt fleet renewal",
  stage: "Negotiation",
  amount: "€48,000",
  company: "Brandt Automotive",
  owner: null,
  source: "Website",
};

const dealColumns: readonly ListColumn<Deal>[] = [
  { key: "name", header: "Deal", cell: (row) => row.name, fixed: true },
  { key: "source", header: "Source", cell: (row) => row.source },
  {
    key: "company",
    header: "Company",
    cell: (row) => row.company,
    phone: "caption",
  },
  {
    key: "owner",
    header: "Owner",
    cell: (row) => row.owner ?? "Unowned",
    phone: "caption",
    folded: (row) => (row.owner ? undefined : null),
  },
  {
    key: "amount",
    header: "Amount",
    cell: (row) => row.amount,
    phone: "caption",
  },
  { key: "stage", header: "Stage", cell: (row) => row.stage, phone: "end" },
  {
    key: "menu",
    header: "Actions",
    verbs: "menu",
    cell: (row) => <button type="button">{`Actions for ${row.name}`}</button>,
  },
];

function deals(rows: readonly Deal[], columns = dealColumns) {
  return render(
    <ListTable
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="deals"
    />,
  );
}

function rowOf(name: string) {
  return phoneRow(screen.getByText(name));
}

function cellsOf(name: string): HTMLElement[] {
  return within(rowOf(name).row).getAllByRole("cell");
}

function Nothing() {
  return null;
}

describe("a phone row", () => {
  it("draws the identity, the end item and the menu, then one caption of facts", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, owner: "Mia" }]);
    const cells = cellsOf(deal.name);
    expect(cells.map((cell) => cell.dataset.phone)).toEqual([
      undefined,
      "end",
      "menu",
      "caption",
    ]);
    // A table laid out as blocks keeps its roles only where the markup names them.
    for (const cell of cells) {
      expect(cell).toHaveAttribute("role", "cell");
    }
    const row = rowOf(deal.name);
    expect(row.identity).toHaveTextContent(deal.name);
    expect(row.end).toEqual(["Negotiation"]);
    expect(row.caption).toEqual(["Brandt Automotive", "Mia", "€48,000"]);
    expect(screen.queryByText("Website")).toBeNull();
    expect(screen.getByRole("table")).toHaveClass("lt-phonerows");
  });

  it("reads each fact after its column's header", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, owner: "Mia" }]);
    expect(cellsOf(deal.name)[3]).toHaveAccessibleName(
      "Company Brandt Automotive Owner Mia Amount €48,000",
    );
  });

  it("leaves out a fact whose cell only stands in for nothing", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, company: null }]);
    expect(rowOf(deal.name).caption).toEqual(["€48,000"]);
    expect(screen.queryByText("Unowned")).toBeNull();
  });

  it("draws a fact whose component renders nothing as an empty one, which CSS leaves out", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    const partner: ListColumn<Deal> = {
      key: "partner",
      header: "Partner",
      cell: () => <Nothing />,
      phone: "caption",
    };
    deals([deal], [...dealColumns.slice(0, 2), partner]);
    const caption = rowOf(deal.name).row.querySelector(
      '[data-phone="caption"]',
    );
    expect(caption?.querySelectorAll(".lt-fact")).toHaveLength(1);
    expect(caption?.querySelector(".lt-fact:empty")).not.toBeNull();
  });

  it("says what `folded` gives in place of the cell, the identity's included", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    const columns: ListColumn<Deal>[] = [
      {
        key: "name",
        header: "Deal",
        fixed: true,
        cell: (row) => `${row.name} · ${row.source}`,
        folded: (row) => row.name,
      },
      {
        key: "stage",
        header: "Stage",
        cell: (row) => `Stage: ${row.stage}`,
        phone: "end",
        folded: (row) => row.stage.slice(0, 3),
      },
    ];
    deals([deal], columns);
    const row = rowOf(deal.name);
    expect(row.identity).toHaveTextContent(new RegExp(`^${deal.name}$`));
    expect(row.end).toEqual(["Neg"]);
  });

  it("draws one end item, the first one declared", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    const place = (key: string): ListColumn<Deal> => ({
      key,
      header: key,
      cell: (row) => (key === "stage" ? row.stage : row.amount),
      phone: "end",
    });
    deals([deal], [dealColumns[0], place("stage"), place("amount")]);
    expect(rowOf(deal.name).end).toEqual(["Negotiation"]);
  });

  it("draws two bones a row while the first page loads", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    render(
      <ListTable
        rows={[]}
        pending
        columns={dealColumns}
        rowKey={(row) => row.id}
        unit="deals"
      />,
    );
    const loading = document.querySelector("tr.lt-loading");
    expect(loading?.querySelectorAll(".lt-bone")).toHaveLength(2);
  });

  it("is a desktop row above the fold, every column in its place with its label", () => {
    stubWindowWidth(FOLD_MAX_WIDTH + 1);
    deals([deal]);
    const cells = cellsOf(deal.name);
    expect(cells.map((cell) => cell.textContent)).toEqual([
      deal.name,
      "Website",
      "Brandt Automotive",
      "Unowned",
      "€48,000",
      "Negotiation",
      `Actions for ${deal.name}`,
    ]);
    expect(cells.map((cell) => cell.dataset.label)).toEqual([
      undefined,
      "Source",
      "Company",
      "Owner",
      "Amount",
      "Stage",
      "Actions",
    ]);
    expect(screen.getByRole("table")).not.toHaveClass("lt-phonerows");
  });
});

describe("a list that places no column on a phone", () => {
  const unplaced: ListColumn<Deal>[] = dealColumns.map((column) => ({
    key: column.key,
    header: column.header,
    cell: column.cell,
    fixed: column.fixed,
    verbs: column.verbs,
  }));

  it("keeps the labelled card: every shown column, each with its label", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([deal], unplaced);
    const cells = cellsOf(deal.name);
    expect(cells).toHaveLength(unplaced.length);
    expect(cells.slice(1).map((cell) => cell.dataset.label)).toEqual(
      unplaced.slice(1).map((column) => column.header),
    );
    expect(screen.getByRole("table")).not.toHaveClass("lt-phonerows");
  });

  it("keeps the Columns step", async () => {
    const user = userEvent.setup();
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([deal], unplaced);
    const panel = within(await openView(user));
    expect(
      panel.getByRole("button", { name: "Columns, 6 of 6" }),
    ).toBeInTheDocument();
  });
});

describe("a phone-only column", () => {
  const since: ListColumn<Deal> = {
    key: "since",
    header: "Lead source",
    cell: (row) => row.source,
    phone: "caption",
    phoneOnly: true,
  };

  it("is a caption fact on a phone", () => {
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([deal], [dealColumns[0], since]);
    expect(rowOf(deal.name).caption).toEqual(["Website"]);
  });

  it("is no column, tick or header above the fold", async () => {
    const user = userEvent.setup();
    stubWindowWidth(FOLD_MAX_WIDTH + 1);
    deals([deal], [...dealColumns.slice(0, 2), since]);
    expect(
      screen.queryByRole("columnheader", { name: "Lead source" }),
    ).toBeNull();
    expect(cellsOf(deal.name)).toHaveLength(2);
    const panel = within(await openView(user));
    expect(
      panel.getByRole("button", { name: "Columns, 1 of 1" }),
    ).toBeInTheDocument();
  });
});

describe("the View panel on a phone", () => {
  it("offers no Columns step to phone rows, which the wider window keeps", async () => {
    const user = userEvent.setup();
    stubWindowWidth(FOLD_MAX_WIDTH);
    deals([deal]);
    const phonePanel = within(await openView(user));
    expect(phonePanel.queryByRole("button", { name: /^Columns/ })).toBeNull();
    expect(
      phonePanel.getByRole("button", { name: "Compact" }),
    ).toBeInTheDocument();

    cleanup();
    stubWindowWidth(FOLD_MAX_WIDTH + 1);
    deals([deal]);
    const deskPanel = within(await openView(user));
    expect(
      deskPanel.getByRole("button", { name: "Columns, 6 of 6" }),
    ).toBeInTheDocument();
  });

  // A window that crosses the fold: every `max-width` query answers again.
  function stubLiveWidth(start: number) {
    let width = start;
    const listeners = new Set<() => void>();
    vi.stubGlobal("matchMedia", (query: string) => ({
      get matches() {
        return width <= Number(/max-width:\s*(\d+)px/.exec(query)?.[1] ?? 0);
      },
      media: query,
      addEventListener: (_: string, listener: () => void) =>
        listeners.add(listener),
      removeEventListener: (_: string, listener: () => void) =>
        listeners.delete(listener),
    }));
    return (next: number) =>
      act(() => {
        width = next;
        for (const listener of listeners) listener();
      });
  }

  it("takes an open Columns step back to the panel's rows when the window narrows", async () => {
    const user = userEvent.setup();
    const resize = stubLiveWidth(1280);
    deals([deal]);
    await openViewStep(user, "Columns");

    resize(390);
    expect(screen.getByRole("table")).toHaveClass("lt-phonerows");
    const panel = await screen.findByRole("dialog");
    expect(within(panel).queryByRole("group", { name: "Columns" })).toBeNull();
    expect(panel.contains(document.activeElement)).toBe(true);

    resize(1280);
    expect(screen.queryByRole("group", { name: "Columns" })).toBeNull();
  });

  it("forgets a step whose row leaves, and hands focus to the rows left", async () => {
    const user = userEvent.setup();
    stubWindowWidth(1280);
    const dial = (key: string, label: string): ViewRow => ({
      key,
      label,
      value: "",
      step: () => <p>{`${label} step`}</p>,
    });
    const menu = (rows: readonly ViewRow[]) => (
      <ListViewMenu
        open
        onOpenChange={() => undefined}
        filterCount={0}
        changed={false}
        rows={rows}
      />
    );
    const both = [dial("sort", "Sort"), dial("columns", "Columns")];
    const { rerender } = render(menu(both));
    await user.click(screen.getByRole("button", { name: /^Columns/ }));
    expect(screen.getByText("Columns step")).toBeInTheDocument();

    rerender(
      <LocaleProvider initial="en">{menu(both.slice(0, 1))}</LocaleProvider>,
    );
    expect(document.activeElement).toHaveAccessibleName(/^Sort/);
    rerender(<LocaleProvider initial="en">{menu(both)}</LocaleProvider>);
    expect(screen.queryByText("Columns step")).toBeNull();
  });
});
