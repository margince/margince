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

describe("a phone row", () => {
  it("draws the identity, the end item, the menu and the caption facts, in that order", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, owner: "Mia" }]);
    const cells = cellsOf(deal.name);
    expect(cells.map((cell) => cell.textContent)).toEqual([
      deal.name,
      "Negotiation",
      `Actions for ${deal.name}`,
      "Brandt Automotive",
      "Mia",
      "€48,000",
    ]);
    expect(cells.map((cell) => cell.dataset.phone)).toEqual([
      undefined,
      "end",
      undefined,
      "caption",
      "caption",
      "caption",
    ]);
    expect(screen.queryByText("Website")).toBeNull();
  });

  it("leaves out a fact whose cell is empty or only stands in for nothing", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals([{ ...deal, company: null }]);
    const cells = cellsOf(deal.name);
    const [company, owner, amount] = cells.slice(3);
    expect(company.dataset.phone).toBe("blank");
    expect(owner.dataset.phone).toBe("blank");
    expect(owner).toBeEmptyDOMElement();
    expect(screen.queryByText("Unowned")).toBeNull();
    expect(amount.dataset.phone).toBe("caption");
  });

  it("says what `folded` gives in place of the cell", () => {
    stubWidth(FOLD_MAX_WIDTH);
    const shortStage: ListColumn<Deal> = {
      key: "stage",
      header: "Stage",
      cell: (row) => `Stage: ${row.stage}`,
      phone: "end",
      folded: (row) => row.stage.slice(0, 3),
    };
    deals([deal], [dealColumns[0], shortStage]);
    expect(cellsOf(deal.name)[1]).toHaveTextContent(/^Neg$/);
  });

  it("draws one end item, the first one declared", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals(
      [deal],
      [
        dealColumns[0],
        {
          key: "stage",
          header: "Stage",
          cell: (row) => row.stage,
          phone: "end",
        },
        {
          key: "amount",
          header: "Amount",
          cell: (row) => row.amount,
          phone: "end",
        },
      ],
    );
    expect(cellsOf(deal.name).map((cell) => cell.textContent)).toEqual([
      deal.name,
      "Negotiation",
    ]);
  });

  it("holds the identity and the menu alone when no column has a phone place", () => {
    stubWidth(FOLD_MAX_WIDTH);
    deals(
      [deal],
      dealColumns.map(({ phone: _place, ...column }) => column),
    );
    expect(cellsOf(deal.name).map((cell) => cell.textContent)).toEqual([
      deal.name,
      `Actions for ${deal.name}`,
    ]);
  });

  it("is a desktop row above the fold, every column in its place and unmarked", () => {
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
    for (const cell of cells) {
      expect(cell).not.toHaveAttribute("data-phone");
    }
  });
});

describe("the View panel on a phone", () => {
  it("offers no Columns step, which the wider window keeps", async () => {
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
