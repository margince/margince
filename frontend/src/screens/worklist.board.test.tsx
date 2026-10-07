// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { jsonResponse, render, stubApi } from "./brief.testkit";
import { TeamBoard } from "./worklist.board";

// A named team lists the seats that have not signed in yet. Their counts were
// never measured, and "—" already means zero on this board, so an invited row
// must say neither a figure nor a dash.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const lead = {
  user_id: "11111111-1111-4111-8111-111111111111",
  display_name: "Lena Fischer",
  activation: "active",
  counts: { waiting: 3, at_risk: 0, overdue: 1, promises_due: 0 },
};
const seat = {
  user_id: "22222222-2222-4222-8222-222222222222",
  display_name: "Ana Novak",
  activation: "invited",
  counts: { waiting: 0, at_risk: 0, overdue: 0, promises_due: 0 },
};
const nothingUnowned = { waiting: 0, at_risk: 0, overdue: 0, promises_due: 0 };

async function rowOf(name: string, members: readonly object[]) {
  stubApi({
    "GET /worklist/team": () =>
      jsonResponse({
        as_of: "2026-06-10T06:00:00Z",
        members,
        unassigned: nothingUnowned,
        truncated: false,
      }),
  });
  render(
    <TeamBoard onOwner={vi.fn()} onUnassigned={vi.fn()} teamId="team-1" />,
  );
  const cell = await screen.findByText(name);
  const row = cell.closest("tr");
  if (!row) {
    throw new Error(`no table row holds ${name}`);
  }
  return row;
}

describe("an invited seat on a named team's board", () => {
  it("draws the lead and the invited seat as the two rows the server sent", async () => {
    await rowOf(seat.display_name, [lead, seat]);

    const rows = screen.getAllByRole("row").slice(1);
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("Lena Fischer"),
      expect.stringContaining("Ana Novak"),
    ]);
  });

  it("marks the seat invited and says each figure was not measured", async () => {
    const row = await rowOf(seat.display_name, [lead, seat]);
    const cells = within(row).getAllByRole("cell");

    expect(within(cells[0]).getByText(en["users.status.invited"])).toBeTruthy();
    for (const cell of cells.slice(1)) {
      expect(cell.textContent).toBe(en["worklist.board.notMeasured"]);
    }
    expect(row.textContent).not.toMatch(/\d|—/);
  });

  // No day to open and no plan to review: a door here would lead to a
  // queue that was never read.
  it("offers no drill-down and no plan review for the invited seat", async () => {
    const row = await rowOf(seat.display_name, [lead, seat]);

    expect(within(row).queryByRole("button")).toBeNull();
  });

  // A server from before activation listed active seats only.
  it("reads a member with no activation as active", async () => {
    const unmarked = {
      user_id: lead.user_id,
      display_name: lead.display_name,
      counts: lead.counts,
    };
    const row = await rowOf(lead.display_name, [unmarked]);

    expect(
      within(row).getByRole("button", { name: lead.display_name }),
    ).toBeTruthy();
    expect(
      within(row).queryByText(en["worklist.board.notMeasured"]),
    ).toBeNull();
  });
});
