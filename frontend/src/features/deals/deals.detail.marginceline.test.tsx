/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DealScreen } from "./deals";
import { deal, jsonResponse, render, stubBackend } from "./deals.testkit";

const CLOSING = "01a00000-0000-7000-8000-0000000000c1";

afterEach(() => {
  cleanup();
  asked.length = 0;
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

function closedByMargince(stageHistoryId: string) {
  return {
    changes: [
      {
        id: "a-1",
        kind: "deal_close_lost",
        summary: "No word from the customer for 196 days",
        occurred_at: "2026-11-20T02:00:00Z",
        subject: { type: "deal", id: "x", label: "Deal" },
        undo: { audit_log_id: "a-1", version: 4, reversed: false },
        review: {
          kind: "close_lost",
          accepted: false,
          reversed: false,
          version: 4,
          can_undo: true,
          can_accept: true,
          writable: true,
          stage_history_id: stageHistoryId,
        },
      },
    ],
  };
}

const asked: string[] = [];

function stubWithChanges(stageHistoryId: string | null) {
  const d = {
    ...deal({ id: "x", status: "lost", stage_id: "s3" }),
    closing_occurrence_id: CLOSING,
  };
  const base = stubBackend([d], { single: d });
  vi.stubGlobal(
    "fetch",
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/deals/x/margince-changes")) {
        asked.push(url);
        return jsonResponse(
          stageHistoryId ? closedByMargince(stageHistoryId) : { changes: [] },
        );
      }
      return base(input, init);
    },
  );
}

describe("DealScreen — the line under the header for a close Margince made", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("names the close with its Undo where the owner reads the deal", async () => {
    stubWithChanges(CLOSING);
    render(<DealScreen id="x" />);
    const line = await screen.findByText(
      /Margince closed this deal as lost on/,
    );
    const callout = line.closest(".callout") as HTMLElement;
    expect(
      await within(callout).findByRole("button", { name: "Undo" }),
    ).toBeTruthy();
    // The close is asked for by the stage move it wrote, so it is found
    // behind any number of later changes.
    expect(
      asked.some((url) => url.includes(`stage_history_id=${CLOSING}`)),
    ).toBe(true);
  });

  it("draws no line, and holds no space for one, on a deal closed by hand", async () => {
    stubWithChanges("01a00000-0000-7000-8000-0000000000c2");
    const { container } = render(<DealScreen id="x" />);
    await screen.findAllByText("Fleet retrofit");
    await waitFor(() =>
      expect(screen.queryByText(/Margince closed this deal/)).toBeNull(),
    );
    expect(container.querySelector(".callout")).toBeNull();
  });
});
