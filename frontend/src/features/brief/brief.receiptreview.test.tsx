/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { ReceiptReview } from "../../components/receiptreview";
import { formatDayMonth, formatTimeOfDay } from "../../format/format";
import { browserZone } from "../../format/timezone";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  pendingSummaryFor,
  render,
  stubApi,
  writes,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "../../testing/worklist.receiptreview.fixtures";
import { DoneForYou } from "./brief.doneforyou";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("marks the exact recorded stage change reviewed and keeps its Undo", async () => {
  let accepted = false;
  const change = automaticStageReceipt;
  const path = `/deals/${change.subject?.id}/applied-changes/${change.id}/accept`;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: change.occurred_at,
        truncated: false,
        receipts: [{ ...change, review: { ...change.review, accepted } }],
      }),
    "GET /deal-corrections/pending": () =>
      jsonResponse(pendingSummaryFor(accepted ? [] : [change])),
    [`GET /deals/${change.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
    [`POST ${path}`]: () => {
      accepted = true;
      return new Response(null, { status: 204 });
    },
  });
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  const press = await screen.findByRole("button", { name: "Mark reviewed" });
  const magicReads = () =>
    calls.filter((call) => call.method === "GET" && call.path === "/magic")
      .length;
  const readBefore = magicReads();
  await user.click(press);
  // The row stays with its answer, and a reviewed change still undoes.
  expect(await screen.findByText(en["brief.changes.reviewed"])).toBeTruthy();
  expect(screen.getByRole("button", { name: "Undo" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Mark reviewed" })).toBeNull();
  // The done lane is read again, so the same act is not left showing there.
  await waitFor(() => expect(magicReads()).toBeGreaterThan(readBefore));
  expect(writes(calls).map((call) => call.path)).toEqual([path]);
});

it("draws no group at all when no change waits for a word", async () => {
  stubApi({
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts: [],
      }),
  });
  const { container, client } = render(<DoneForYou completed={null} />);
  // It draws nothing while loading too. So every empty answer, the receipts,
  // Margince's changes and the done lane, has to come back before "nothing"
  // means anything.
  await waitFor(() =>
    expect(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.status),
    ).toEqual(["success", "success", "success"]),
  );
  // An empty section would be a heading over nothing.
  expect(container.innerHTML).toBe("");
  expect(
    screen.queryByRole("heading", { name: en["brief.done.title"] }),
  ).toBeNull();
});

it("dates a change from today by its hour and an older one by its day", async () => {
  const today = automaticStageReceipt;
  const older = {
    ...automaticStageReceipt,
    id: "01a00000-0000-7000-8000-000000000013",
    occurred_at: "2026-09-09T14:00:00Z",
  };
  stubApi({
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: today.occurred_at,
        truncated: false,
        receipts: [today, older],
      }),
    "GET /deal-corrections/pending": () =>
      jsonResponse(pendingSummaryFor([today, older])),
    [`GET /deals/${today.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
  });
  const { container } = render(<DoneForYou completed={null} />);
  await screen.findByRole("list", { name: en["brief.done.dealsLabel"] });
  const zone = browserZone();
  expect(
    [...container.querySelectorAll("time")].map((time) => time.textContent),
  ).toEqual([
    formatTimeOfDay(today.occurred_at, "en", zone),
    formatDayMonth(older.occurred_at, "en", zone),
  ]);
});

it("offers no write for a superseded change", () => {
  stubApi({});
  render(
    <ReceiptReview
      receipt={{
        ...automaticStageReceipt,
        review: {
          kind: "stage",
          accepted: false,
          reversed: false,
          version: 8,
          can_accept: false,
          writable: true,
          can_undo: false,
        },
      }}
    />,
  );
  expect(screen.queryByRole("button")).toBeNull();
});

it("keeps an undo-only applied change visible until it is reversed", async () => {
  const { review, ...change } = automaticDateReceipt;
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: change.occurred_at,
        truncated: false,
        receipts: [change],
      }),
    "GET /deal-corrections/pending": () =>
      jsonResponse(pendingSummaryFor([change])),
    [`GET /deals/${change.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
  });
  render(<DoneForYou completed={null} />);
  expect(await screen.findByRole("button", { name: "Undo" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Mark reviewed" })).toBeNull();
});

it("asks before an opened deal's receipt archives the team's work, as the change list does", async () => {
  const opened = {
    ...automaticStageReceipt,
    kind: "deal_open",
    review: automaticStageReceipt.review && {
      ...automaticStageReceipt.review,
      kind: "deal_open" as const,
    },
  };
  let asked = 0;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () => {
      asked += 1;
      return jsonResponse({
        results: [
          asked === 1
            ? {
                deal_id: opened.subject?.id,
                outcome: "refused",
                undone: [],
                kept: [
                  {
                    audit_log_id: opened.undo?.audit_log_id,
                    kind: "deal_open",
                    reason: "needs_confirmation",
                    team_notes: 1,
                    team_tasks: 0,
                    team_work: "w1",
                  },
                ],
              }
            : {
                deal_id: opened.subject?.id,
                outcome: "undone",
                undone: ["deal_open"],
                kept: [],
              },
        ],
      });
    },
  });
  const user = userEvent.setup();
  render(<ReceiptReview receipt={opened} />);
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  expect(
    await screen.findByText(
      "This deal now has 1 notes and 0 tasks from your team. Archive it anyway?",
    ),
  ).toBeTruthy();
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  expect(await screen.findByText(en["changes.undo.archived"])).toBeTruthy();
  expect(
    writes(calls)
      .filter((call) => call.path === "/deal-corrections/undo")
      .map((call) => call.body),
  ).toEqual([
    {
      items: [
        {
          deal_id: opened.subject?.id,
          version: 7,
          audit_log_ids: [opened.undo?.audit_log_id],
        },
      ],
    },
    {
      items: [
        {
          deal_id: opened.subject?.id,
          version: 7,
          audit_log_ids: [opened.undo?.audit_log_id],
          archive_confirmed_for: "w1",
        },
      ],
    },
  ]);
});

it("asks before a reopened deal's receipt closes the team's work as lost, then says it is lost again", async () => {
  const reopened = {
    ...automaticStageReceipt,
    kind: "deal_revive",
    review: automaticStageReceipt.review && {
      ...automaticStageReceipt.review,
      kind: "deal_revive" as const,
    },
  };
  let asked = 0;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () => {
      asked += 1;
      return jsonResponse({
        results: [
          asked === 1
            ? {
                deal_id: reopened.subject?.id,
                outcome: "refused",
                undone: [],
                kept: [
                  {
                    audit_log_id: reopened.undo?.audit_log_id,
                    kind: "deal_revive",
                    reason: "needs_confirmation",
                    team_notes: 1,
                    team_tasks: 0,
                    team_work: "w1",
                  },
                ],
              }
            : {
                deal_id: reopened.subject?.id,
                outcome: "undone",
                undone: ["deal_revive"],
                kept: [],
              },
        ],
      });
    },
  });
  const user = userEvent.setup();
  render(<ReceiptReview receipt={reopened} />);
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  expect(
    await screen.findByText(
      "This deal now has 1 notes and 0 tasks from your team. Close it as lost anyway?",
    ),
  ).toBeTruthy();
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  expect(await screen.findByText(en["changes.undo.relost"])).toBeTruthy();
  expect(
    writes(calls)
      .filter((call) => call.path === "/deal-corrections/undo")
      .map((call) => call.body),
  ).toEqual([
    {
      items: [
        {
          deal_id: reopened.subject?.id,
          version: 7,
          audit_log_ids: [reopened.undo?.audit_log_id],
        },
      ],
    },
    {
      items: [
        {
          deal_id: reopened.subject?.id,
          version: 7,
          audit_log_ids: [reopened.undo?.audit_log_id],
          archive_confirmed_for: "w1",
        },
      ],
    },
  ]);
});
