// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { act, cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { MagicPanel } from "../sign-in/magic";
import type { MagicLine, MagicReceipt } from "../sign-in/magic.queries";
import { HandledForYouPanel } from "./worklist.handled";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const AS_OF = "2026-09-13T09:00:00Z";

// One decision an audit row and an approval both stand for, one grouped job,
// and one undone lookup. The old two lists split these kinds.
function doneLines(): MagicLine[] {
  const system = {
    type: "system" as const,
    id: "system",
    label: { key: "magic.by.system" },
  };
  return [
    {
      id: "01a00000-0000-7000-8000-0000000000a1",
      receipt_ids: [automaticStageReceipt.id],
      occurred_at: automaticStageReceipt.occurred_at,
      lane: "done",
      summary: { key: "magic.action.advance_stage" },
      actor: system,
      entity: {
        type: "deal",
        id: automaticStageReceipt.subject?.id ?? "",
        label: "PIM Rollout",
      },
    },
    {
      id: "01a00000-0000-7000-8000-0000000000a2",
      receipt_ids: [
        "01a00000-0000-7000-8000-0000000000b1",
        "01a00000-0000-7000-8000-0000000000b2",
      ],
      occurred_at: "2026-09-13T06:00:00Z",
      lane: "done",
      count: 2,
      summary: { key: "magic.action.mail_filed" },
      actor: system,
    },
    {
      id: "01a00000-0000-7000-8000-0000000000a3",
      receipt_ids: ["01a00000-0000-7000-8000-0000000000a3"],
      occurred_at: "2026-09-13T05:00:00Z",
      lane: "done",
      summary: { key: "magic.action.company_profile_read" },
      actor: system,
      entity: { type: "company", id: "c1", label: "Lookup GmbH" },
      undo: { undoable: false, reason: "already_undone" },
    },
  ];
}

function receiptOver(since: string, done: MagicLine[]): MagicReceipt {
  return {
    as_of: AS_OF,
    since,
    done,
    needs_you: [],
    could_not_complete: [],
    watching: [],
    not_shown: [],
    sources_unavailable: [],
    totals: {
      done: done.length,
      needs_you: 0,
      could_not_complete: 0,
      watching: 0,
    },
  };
}

function stubBoth(
  since: string,
  done: MagicLine[],
  overrides: Partial<Record<string, () => Response>> = {},
  magic: Partial<MagicReceipt> = {},
) {
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /magic": () => jsonResponse({ ...receiptOver(since, done), ...magic }),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: AS_OF,
        since,
        truncated: false,
        receipts: [automaticStageReceipt],
      }),
    ...overrides,
  });
}

const failure = () =>
  new Response(JSON.stringify({ title: "unavailable" }), {
    status: 502,
    headers: { "content-type": "application/problem+json" },
  });

// What one surface shows of Done for you: its lines in order and its count.
async function drawnBy(ui: React.ReactElement) {
  const { container, unmount } = render(ui);
  const list = await screen.findByRole("list", { name: en["magic.lane.done"] });
  // The review rides on the line once the worklist's receipt read answers.
  await within(list).findByRole("button", { name: en["brief.changes.accept"] });
  const lines = within(list)
    .getAllByRole("listitem")
    .map((item) => item.textContent ?? "");
  const fold =
    container.querySelector(".magic-fold summary")?.textContent ?? "";
  const accepts = within(list).queryAllByRole("button", {
    name: en["brief.changes.accept"],
  }).length;
  unmount();
  return { lines, fold, accepts };
}

describe("Done for you is one list on Home and on the Worklist", () => {
  it.each([
    ["the last brief 2 hours ago", "2026-09-13T07:00:00Z"],
    ["the last brief 48 hours ago", "2026-09-11T09:00:00Z"],
  ])("draws the same lines, count and window with %s", async (_, since) => {
    stubBoth(since, doneLines());
    const home = await drawnBy(<MagicPanel />);
    const worklist = await drawnBy(<HandledForYouPanel />);
    expect(worklist).toEqual(home);
    // Every line is drawn, the decision once, with its review on the line.
    expect(home.lines).toHaveLength(3);
    expect(home.accepts).toBe(1);
    expect(home.fold).toBe("All 4 changes, one by one");
  });

  it("keeps an undone change in the list with its status", async () => {
    stubBoth("2026-09-13T07:00:00Z", doneLines());
    render(<HandledForYouPanel />);
    expect(
      await screen.findByText(en["history.undo.alreadyUndone"]),
    ).toBeTruthy();
  });

  it("says nothing was done over a read that answered with no lines", async () => {
    stubBoth("2026-09-13T07:00:00Z", []);
    render(<HandledForYouPanel />);
    expect(await screen.findByText(en["worklist.handled.empty"])).toBeTruthy();
  });

  it("says the list could not be read rather than reporting a clear day", async () => {
    stubApi({ "GET /magic": failure });
    render(<HandledForYouPanel />);
    expect(await screen.findByText(en["state.failed"])).toBeTruthy();
    expect(screen.queryByText(en["worklist.handled.empty"])).toBeNull();
  });

  // Only a complete empty read may say nothing was done.
  it.each([
    [
      "a source withheld",
      {
        sources_unavailable: [
          { source: "approval", reason: "withheld" as const },
        ],
      },
      ".magic-withheld li",
    ],
    [
      "changes this read could not place",
      { not_shown: [{ reason: "unknown_entity_type" as const, count: 3 }] },
      ".magic-notshown li",
    ],
  ])(
    "says the read is incomplete, not empty, with %s",
    async (_, magic, marker) => {
      stubBoth("2026-09-13T07:00:00Z", [], {}, magic);
      const { container } = render(<HandledForYouPanel />);
      await screen.findByRole("heading", { name: en["magic.lane.done"] });
      await vi.waitFor(() =>
        expect(container.querySelector(marker)).toBeTruthy(),
      );
      expect(screen.queryByText(en["worklist.handled.empty"])).toBeNull();
    },
  );

  it("says the lane did not load when the answer carried no lane at all", async () => {
    stubBoth("2026-09-13T07:00:00Z", [], {}, { done: undefined });
    render(<HandledForYouPanel />);
    expect(await screen.findByText(en["state.unavailable"])).toBeTruthy();
    expect(screen.queryByText(en["worklist.handled.empty"])).toBeNull();
  });

  // Home and the Worklist read one incomplete answer the same way.
  it.each([
    [
      "no Done lane in the answer",
      { done: undefined },
      en["state.unavailable"],
    ],
    [
      "a withheld count",
      { not_shown_count_withheld: true },
      en["magic.visibleScope"],
    ],
  ])("neither surface calls %s a quiet day", async (_, magic, explanation) => {
    for (const ui of [
      <MagicPanel key="home" />,
      <HandledForYouPanel key="worklist" />,
    ]) {
      stubBoth("2026-09-13T07:00:00Z", [], {}, magic);
      const { unmount } = render(ui);
      expect(await screen.findByText(explanation)).toBeTruthy();
      expect(screen.queryByText(en["worklist.handled.empty"])).toBeNull();
      unmount();
      vi.unstubAllGlobals();
    }
  });

  it("calls a complete empty read a quiet day", async () => {
    stubBoth("2026-09-13T07:00:00Z", []);
    render(<HandledForYouPanel />);
    expect(await screen.findByText(en["worklist.handled.empty"])).toBeTruthy();
    expect(screen.queryByText(en["magic.visibleScope"])).toBeNull();
  });

  it("withdraws a cached review once its read fails on refetch", async () => {
    stubBoth("2026-09-13T07:00:00Z", doneLines());
    const { client } = render(<HandledForYouPanel />);
    expect(
      await screen.findByRole("button", { name: en["brief.changes.accept"] }),
    ).toBeTruthy();
    stubBoth("2026-09-13T07:00:00Z", doneLines(), {
      "GET /worklist/handled": failure,
    });
    await act(async () => {
      await client.refetchQueries({ queryKey: ["worklist", "handled"] });
    });
    expect(
      await screen.findByText(en["magic.review.unavailable"]),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["brief.changes.accept"] }),
    ).toBeNull();
  });

  it("says a review past the receipt read's bound did not load", async () => {
    stubBoth("2026-09-13T07:00:00Z", doneLines(), {
      "GET /worklist/handled": () =>
        jsonResponse({ as_of: AS_OF, truncated: true, receipts: [] }),
    });
    render(<HandledForYouPanel />);
    expect(
      await screen.findByText(en["magic.review.unavailable"]),
    ).toBeTruthy();
  });

  it("offers no review a revoked grant no longer allows", async () => {
    stubBoth("2026-09-13T07:00:00Z", doneLines(), {
      "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    });
    render(<HandledForYouPanel />);
    await screen.findByRole("list", { name: en["magic.lane.done"] });
    await vi.waitFor(() =>
      expect(screen.queryByText(en["magic.review.unavailable"])).toBeNull(),
    );
    expect(
      screen.queryByRole("button", { name: en["brief.changes.accept"] }),
    ).toBeNull();
  });
});
