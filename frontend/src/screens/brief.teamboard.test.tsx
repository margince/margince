/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefTeamBoard } from "./brief.teamboard";
import { jsonResponse, render, stubApi } from "./brief.testkit";

// The team board on Brief. It is the SAME component the Worklist draws, so what
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

/**
 * The board drawn on Brief, settled, with the driver its rows are pressed with.
 *
 * The heading is asserted here rather than in a frame of its own: it is the
 * precondition every test below reads through — the board is an open titled
 * panel like the blocks around it, so a row is on screen without anything
 * being opened first.
 */
async function drawBoard() {
  const user = userEvent.setup();
  render(<BriefTeamBoard offered />);
  await screen.findByRole("heading", { name: en["worklist.board.title"] });
  return user;
}

describe("the team board on Brief", () => {
  // A rep never sees it, and the read is never made. Drawing a control on a
  // tier the server refuses is a control that exists to fail.
  it("draws nothing and asks nothing for a reader whose scope reaches no team", () => {
    const calls = stubApi({});
    const { container } = render(<BriefTeamBoard offered={false} />);

    expect(container.firstChild).toBeNull();
    expect(calls.filter((call) => call.path === "/worklist/team")).toHaveLength(
      0,
    );
  });

  // The row is a door to that contact's day. Without the id in the address it
  // could only reach the Worklist, leaving the reader to pick the same contact
  // a second time — a row that answers a question by asking it again.
  it("opens a colleague's own queue by name in the address", async () => {
    stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    const user = await drawBoard();

    await user.click(await screen.findByText("Lena Fischer"));

    expect(globalThis.location.hash).toBe(
      "#/worklist/11111111-1111-4111-8111-111111111111",
    );
  });

  // The unowned pile has no contact to open, so the same segment carries the
  // scope word. Both rows are doors, not one door and one shrug.
  it("opens the unassigned pile by its scope word", async () => {
    stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    const user = await drawBoard();

    await user.click(await screen.findByText(en["worklist.board.nobody"]));

    expect(globalThis.location.hash).toBe("#/worklist/unassigned");
  });

  // One read, one query key. Brief and the Worklist cannot report different
  // counts for the same morning because they are not two reads.
  it("reads the board through the shared key, not a second endpoint", async () => {
    const calls = stubApi({ "GET /worklist/team": () => jsonResponse(board) });
    await drawBoard();

    await screen.findByText("Lena Fischer");
    const boardReads = calls.filter((call) => call.path === "/worklist/team");
    expect(boardReads).toHaveLength(1);
    expect(boardReads[0].method).toBe("GET");
  });
});
