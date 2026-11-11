/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { keepUndoManifest } from "../lib/marginchanges.runs";
import { forgetSeat } from "../lib/storage";
import { jsonResponse, render, stubApi } from "../testing/brief.testkit";
import { LastBulkUndo } from "./marginchanges";

afterEach(() => {
  cleanup();
  forgetSeat();
  vi.unstubAllGlobals();
});

it("shows saved results after a fresh mount without sending the Undo again", async () => {
  const items = [1, 2].map((id) => ({
    deal_id: `deal-${id}`,
    version: 7,
    audit_log_ids: [`change-${id}`],
  }));
  const manifest = keepUndoManifest(items);
  const calls = stubApi({
    "POST /deal-corrections/undo/results": () =>
      jsonResponse({
        runs: [
          {
            run_id: manifest.batches[0].id,
            created_at: "2026-09-13T02:00:00Z",
            items,
            complete: true,
            results: items.map((item) => ({
              deal_id: item.deal_id,
              outcome: "undone",
              undone: ["close_lost"],
              kept: [],
            })),
          },
        ],
      }),
  });
  render(<LastBulkUndo />);
  expect(await screen.findByText("Undone on 2 deals.")).toBeTruthy();
  cleanup();
  render(<LastBulkUndo />);
  expect(await screen.findByText("Undone on 2 deals.")).toBeTruthy();
  expect(
    calls.filter((call) => call.path === "/deal-corrections/undo/results"),
  ).toHaveLength(2);
  expect(calls.some((call) => call.path === "/deal-corrections/undo")).toBe(
    false,
  );
  expect(screen.queryByRole("button", { name: "Send again" })).toBeNull();
});
