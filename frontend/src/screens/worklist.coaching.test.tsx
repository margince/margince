// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { COACHING_MOVES, CoachingMoves, movesFor } from "./worklist.coaching";
import type { TeamBoardMember } from "./worklist.queries";

// Which conversations a lead should have this morning.
//
// The board says who is carrying what. These say what to do about it, and the
// claim that costs most if it is wrong is the ORDER: a rep behind on tasks is
// late, and a rep who broke a promise has spent something the customer already
// counted on. Getting that backwards sends a lead to the wrong desk.

function member(
  name: string,
  counts: Partial<TeamBoardMember["counts"]>,
): TeamBoardMember {
  return {
    user_id: `id-${name}`,
    display_name: name,
    counts: { waiting: 0, at_risk: 0, overdue: 0, promises_due: 0, ...counts },
  };
}

function draw(members: readonly TeamBoardMember[], onOwner = vi.fn()) {
  render(
    <LocaleProvider initial="en">
      <CoachingMoves members={members} onOwner={onOwner} />
    </LocaleProvider>,
  );
  return onOwner;
}

afterEach(cleanup);

describe("what a lead should do about the board", () => {
  it("draws at most three moves, each carrying the number it was drawn from", () => {
    draw([
      member("Ana", { promises_due: 2 }),
      member("Ben", { promises_due: 4 }),
      member("Cara", { waiting: 9 }),
      member("Dev", { overdue: 7 }),
      member("Eve", { promises_due: 1 }),
    ]);

    const moves = screen.getAllByRole("button");
    expect(moves).toHaveLength(COACHING_MOVES);
    // Every line says a number. A suggestion a lead cannot check is one they
    // stop reading.
    for (const move of moves) {
      expect(move.textContent).toMatch(/\d/);
    }
  });

  it("puts a broken promise ahead of a bigger queue", () => {
    draw([member("Ana", { waiting: 40 }), member("Ben", { promises_due: 1 })]);

    const first = screen.getAllByRole("button")[0];
    expect(first.textContent).toContain("Ben");
  });

  it("names a person once, however many thresholds they cross", () => {
    const moves = movesFor([
      member("Ana", { promises_due: 3, waiting: 20, overdue: 12 }),
      member("Ben", { waiting: 9 }),
    ]);

    expect(moves.filter((m) => m.name === "Ana")).toHaveLength(1);
    // And the second person still gets their line, which is what one-per-person
    // is FOR: three lines about Ana would push Ben off a list of three.
    expect(moves.map((m) => m.name)).toContain("Ben");
  });

  // A zero is a real answer, not a silence. Claims have a writer on every
  // installation, so a rep at zero owes nothing — and a threshold of one keeps
  // them off the list without the count having to be missing.
  it("says nothing about a person who owes no promises", () => {
    const moves = movesFor([member("Ana", { waiting: 2, overdue: 1 })]);
    expect(moves).toHaveLength(0);
  });

  it("draws nothing at all when nobody is over a threshold", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <CoachingMoves
          members={[member("Ana", { waiting: 1 })]}
          onOwner={vi.fn()}
        />
      </LocaleProvider>,
    );
    expect(container.innerHTML).toBe("");
  });

  it("routes a move to that person's own queue", async () => {
    const user = userEvent.setup();
    const onOwner = draw([member("Ana", { promises_due: 2 })]);

    await user.click(screen.getByRole("button"));

    expect(onOwner).toHaveBeenCalledWith("id-Ana");
  });
});
