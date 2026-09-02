/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { HomeTeamBoard } from "./home.teamboard";
import { jsonResponse, render, stubApi } from "./home.testkit";

// The team board on Home. It is the SAME component the Worklist draws, so what
// these tests are about is not the table — it is where a row goes.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

beforeEach(() => {
  globalThis.location.hash = "";
});

const board = {
  as_of: "2026-06-10T06:00:00Z",
  members: [
    {
      user_id: "11111111-1111-4111-8111-111111111111",
      display_name: "Lena Fischer",
      counts: { waiting: 3, at_risk: 1, overdue: 0 },
    },
  ],
  unassigned: { waiting: 2, at_risk: 0, overdue: 0 },
  truncated: false,
};

async function openBoard() {
  render(<HomeTeamBoard offered />);
  // The board sits behind a disclosure: the reader's own day is what they came
  // for, and a table of colleagues above it would push that off the screen.
  await userEvent.click(await screen.findByText(en["worklist.board.title"]));
}

describe("the team board on Home", () => {
  // A rep never sees it, and the read is never made. Drawing a control on a
  // tier the server refuses is a control that exists to fail.
  it("draws nothing and asks nothing for a reader whose scope reaches no team", () => {
    const calls = stubApi({});
    const { container } = render(<HomeTeamBoard offered={false} />);

    expect(container.firstChild).toBeNull();
    expect(calls.filter((call) => call.path === "/worklist/team")).toHaveLength(
      0,
    );
  });

  // The row is a door to that person's day. Without the id in the address it
  // could only reach the Worklist, leaving the reader to pick the same person
  // a second time — a row that answers a question by asking it again.
  it("opens a colleague's own queue by name in the address", async () => {
    stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    await openBoard();

    await userEvent.click(await screen.findByText("Lena Fischer"));

    expect(globalThis.location.hash).toBe(
      "#/worklist/11111111-1111-4111-8111-111111111111",
    );
  });

  // The unowned pile has no person to open, so the same segment carries the
  // scope word. Both rows are doors, not one door and one shrug.
  it("opens the unassigned pile by its scope word", async () => {
    stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    await openBoard();

    await userEvent.click(await screen.findByText(en["worklist.board.nobody"]));

    expect(globalThis.location.hash).toBe("#/worklist/unassigned");
  });

  // One read, one query key. Home and the Worklist cannot report different
  // counts for the same morning because they are not two reads.
  it("reads the board through the shared key, not a second endpoint", async () => {
    const calls = stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    await openBoard();

    await screen.findByText("Lena Fischer");
    const boardReads = calls.filter((call) => call.path === "/worklist/team");
    expect(boardReads).toHaveLength(1);
    expect(boardReads[0].method).toBe("GET");
  });
});
