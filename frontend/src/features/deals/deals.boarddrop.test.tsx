/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DealsScreen } from "./deals";
import { deal, render, stages, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

const withLost = [
  {
    id: "pl",
    name: "Sales",
    is_default: true,
    position: 0,
    stages: [
      ...stages,
      {
        id: "s4",
        pipeline_id: "pl",
        name: "Lost",
        position: 4,
        semantic: "lost" as const,
        win_probability: 0,
      },
    ],
  },
];

// jsdom carries no drag-and-drop, so the column's drop handler is dispatched
// directly, as the board's drag ends.
function dropOnStage(stageId: string, dealId: string) {
  const column = document.querySelector(
    `[data-stage="${stageId}"]`,
  ) as HTMLElement;
  const dropEvent = new Event("drop", { bubbles: true });
  Object.assign(dropEvent, {
    dataTransfer: { getData: () => dealId, setData: () => {} },
  });
  column.dispatchEvent(dropEvent);
}

async function boardWith(
  dealsOnBoard: Parameters<typeof stubBackend>[0],
  advances: unknown[],
) {
  vi.stubGlobal(
    "fetch",
    stubBackend(dealsOnBoard, {
      pipelines: withLost,
      onAdvance: (body) => advances.push(body),
    }),
  );
  render(<DealsScreen />);
  await waitFor(() =>
    expect(screen.getAllByText(dealsOnBoard[0].name)[0]).toBeTruthy(),
  );
}

describe("a drop on the board", () => {
  it("of a won deal on an open column asks the Reopen question before it moves", async () => {
    const user = userEvent.setup();
    const advances: unknown[] = [];
    await boardWith(
      [deal({ id: "w1", name: "Won one", status: "won", stage_id: "s3" })],
      advances,
    );

    dropOnStage("s2", "w1");

    const dialog = within(await screen.findByRole("dialog"));
    expect(
      dialog.getByText("Move this deal back to an open stage"),
    ).toBeTruthy();
    expect(dialog.getByTestId("reopen-stage-s2")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(advances).toHaveLength(0);

    await user.click(dialog.getByRole("button", { name: "Reopen" }));
    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual({ to_stage_id: "s2", status: "open" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("of a won deal cancelled at the Reopen question sends nothing", async () => {
    const user = userEvent.setup();
    const advances: unknown[] = [];
    await boardWith(
      [deal({ id: "w1", name: "Won one", status: "won", stage_id: "s3" })],
      advances,
    );

    dropOnStage("s1", "w1");
    const dialog = within(await screen.findByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(advances).toHaveLength(0);
  });

  it("of a won deal on the Lost column is refused and says how to move it", async () => {
    const advances: unknown[] = [];
    await boardWith(
      [deal({ id: "w1", name: "Won one", status: "won", stage_id: "s3" })],
      advances,
    );

    dropOnStage("s4", "w1");

    expect(
      await screen.findByText(
        "This deal is closed. Reopen it to move it to another stage.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(advances).toHaveLength(0);
  });

  it("on the column the deal is already in moves nothing", async () => {
    const advances: unknown[] = [];
    await boardWith([deal({ id: "d1", stage_id: "s1" })], advances);

    dropOnStage("s1", "d1");
    // A real move after it: a write for the first drop would be sent first.
    dropOnStage("s2", "d1");

    await waitFor(() => expect(advances).toHaveLength(1));
    expect(advances[0]).toEqual({ to_stage_id: "s2" });
  });
});
