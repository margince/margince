/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { FOLD_MAX_WIDTH } from "../lib/viewport";
import { openView } from "../testing/listview.testkit";
import type { ListColumn } from "./listtable";
import { ListTable } from "./listtable";
import { render } from "./listtable.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubWidth(width: number) {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: width <= Number(/max-width:\s*(\d+)px/.exec(query)?.[1] ?? 0),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

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

function cellsOf(name: string): HTMLElement[] {
  const row = screen.getByText(name).closest("tr");
  if (!row) throw new Error(`no row holds ${name}`);
  return within(row).getAllByRole("cell");
}

function factsOf(name: string): string[] {
  const caption = cellsOf(name).find(
    (cell) => cell.dataset.phone === "caption",
  );
  return [...(caption?.querySelectorAll(".lt-fact") ?? [])].map(
    (fact) => fact.textContent ?? "",
  );
}

describe("a phone row", () => {
  it("draws the identity, the end item and the menu, then one caption of facts", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, owner: "Mia" }]);
    const cells = cellsOf(deal.name);
    expect(cells.map((cell) => cell.dataset.phone)).toEqual([
      undefined,
      "end",
      "menu",
      "caption",
    ]);
    expect(cells[0]).toHaveTextContent(deal.name);
    expect(cells[1]).toHaveTextContent("Negotiation");
    expect(cells[2]).toHaveTextContent(`Actions for ${deal.name}`);
    expect(factsOf(deal.name)).toEqual(["Brandt Automotive", "Mia", "€48,000"]);
    expect(screen.queryByText("Website")).toBeNull();
    expect(screen.getByRole("table")).toHaveClass("lt-phonerows");
  });

  it("leaves out a fact whose cell is empty or only stands in for nothing", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, company: null }]);
    expect(factsOf(deal.name)).toEqual(["€48,000"]);
    expect(screen.queryByText("Unowned")).toBeNull();
  });

  it("draws no caption cell when no fact has anything to say", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, company: null }], dealColumns.slice(0, 4));
    expect(cellsOf(deal.name).map((cell) => cell.dataset.phone)).toEqual([
      undefined,
    ]);
  });

  it("says what `folded` gives in place of the cell, the identity's included", () => {
    stubWidth(FOLD_MAX_WIDTH);
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
    const [identity, stage] = cellsOf(deal.name);
    expect(identity).toHaveTextContent(new RegExp(`^${deal.name}$`));
    expect(stage).toHaveTextContent(/^Neg$/);
  });

  it("draws one end item, the first one declared", () => {
    stubWidth(FOLD_MAX_WIDTH);
    const place = (key: string): ListColumn<Deal> => ({
      key,
      header: key,
      cell: (row) => (key === "stage" ? row.stage : row.amount),
      phone: "end",
    });
    deals([deal], [dealColumns[0], place("stage"), place("amount")]);
    expect(cellsOf(deal.name).map((cell) => cell.textContent)).toEqual([
      deal.name,
      "Negotiation",
    ]);
  });

  it("is a desktop row above the fold, every column in its place with its label", () => {
    stubWidth(FOLD_MAX_WIDTH + 1);
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
  const unplaced = dealColumns.map(({ phone: _place, ...column }) => column);

  it("keeps the labelled card: every shown column, each with its label", () => {
    stubWidth(FOLD_MAX_WIDTH);
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
    stubWidth(FOLD_MAX_WIDTH);
    deals([deal], unplaced);
    const panel = within(await openView(user));
    expect(
      panel.getByRole("button", { name: "Columns, 6 of 6" }),
    ).toBeInTheDocument();
  });
});

describe("a phone-only column", () => {
  const title: ListColumn<Deal> = {
    key: "title",
    header: "Lead source",
    cell: (row) => row.source,
    phone: "caption",
    phoneOnly: true,
  };

  it("is a caption fact on a phone", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([deal], [dealColumns[0], title]);
    expect(factsOf(deal.name)).toEqual(["Website"]);
  });

  it("is no column, tick or header above the fold", async () => {
    const user = userEvent.setup();
    stubWidth(FOLD_MAX_WIDTH + 1);
    deals([deal], [...dealColumns.slice(0, 2), title]);
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
    stubWidth(FOLD_MAX_WIDTH);
    deals([deal]);
    const phonePanel = within(await openView(user));
    expect(phonePanel.queryByRole("button", { name: /^Columns/ })).toBeNull();
    expect(
      phonePanel.getByRole("button", { name: "Compact" }),
    ).toBeInTheDocument();

    cleanup();
    stubWidth(FOLD_MAX_WIDTH + 1);
    deals([deal]);
    const deskPanel = within(await openView(user));
    expect(
      deskPanel.getByRole("button", { name: "Columns, 6 of 6" }),
    ).toBeInTheDocument();
  });
});
