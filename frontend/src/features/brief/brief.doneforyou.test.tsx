/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import type { Receipt } from "../../lib/handled";
import {
  jsonResponse,
  render,
  stubApi,
  writes,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { DONE_PREVIEW_DEALS, DoneForYou } from "./brief.doneforyou";

type Bundle = components["schemas"]["MarginceChangeBundle"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const dealId = (n: number) => `00000000-0000-4000-8000-0000000000${10 + n}`;
const auditId = (n: number, c = 0) =>
  `00000000-0000-7000-8000-00000000${100 + n}${c}`;

// A change on deal n as a current server sends it. The change, its reason and
// the customer's words each travel in a field of their own.
function change(n: number, c = 0, patch: Partial<Receipt> = {}): Receipt {
  const id = auditId(n, c);
  return {
    ...automaticStageReceipt,
    id,
    summary: `Closed "Deal ${n}" as lost — reason ${n}.${c}`,
    headline: `Closed "Deal ${n}" as lost`,
    reason: `reason ${n}.${c}`,
    subject: { type: "deal", id: dealId(n), label: `Deal ${n}` },
    undo: { audit_log_id: id, version: 7, reversed: false },
    ...patch,
  };
}

function bundle(n: number, audits: string[]): Bundle {
  return {
    deal_id: dealId(n),
    deal_name: `Deal ${n}`,
    version: 7,
    audit_log_ids: audits,
    kinds: ["close_lost"],
    latest_at: "2026-09-13T02:00:00Z",
  };
}

function routes(
  receipts: Receipt[],
  bundles: Bundle[],
  undo?: (body: unknown) => Response,
) {
  return stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts,
      }),
    "GET /deal-corrections/pending": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        deals: bundles.length,
        closed: bundles.length,
        redated: 0,
        revalued: 0,
        probabilities: 0,
        moved: 0,
        opened: 0,
        revived: 0,
        base_currency: "EUR",
        bundles,
        truncated: false,
      }),
    ...(undo ? { "POST /deal-corrections/undo": undo } : {}),
  });
}

const six = Array.from({ length: 6 }, (_, i) => i + 1);

it("shows five deals with their reasons, and every one in the review drawer", async () => {
  routes(
    six.map((n) => change(n)),
    six.map((n) => bundle(n, [auditId(n)])),
  );
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  const preview = await screen.findByRole("list", {
    name: en["brief.done.dealsLabel"],
  });
  // Past the preview a deal moves to the drawer whole: never a summary that
  // drops its reason or its verbs.
  for (const n of six.slice(0, DONE_PREVIEW_DEALS)) {
    expect(within(preview).getByText(`reason ${n}.0`)).toBeTruthy();
  }
  expect(within(preview).queryByText("reason 6.0")).toBeNull();
  expect(
    await within(preview).findAllByRole("button", { name: "Mark reviewed" }),
  ).toHaveLength(DONE_PREVIEW_DEALS);
  expect(screen.getByText("6 deals wait for your review")).toBeTruthy();

  await user.click(screen.getByRole("button", { name: "Review all 6 deals" }));
  const drawer = await screen.findByRole("dialog", {
    name: "6 deals Margince changed",
  });
  for (const n of six) {
    expect(within(drawer).getByText(`reason ${n}.0`)).toBeTruthy();
  }
  expect(within(drawer).getAllByRole("button", { name: "Undo" })).toHaveLength(
    6,
  );
  expect(
    within(drawer).getByRole("button", { name: "Undo changes on 6 deals" }),
  ).toBeTruthy();
});

it("reads what changed, why and the customer's words apart", async () => {
  routes(
    [
      change(1, 0, { customer_quote: "We cancelled the project." }),
      change(2, 0, { kind: "deal_close_lost", reason: undefined }),
      change(3, 0, { reason: undefined }),
    ],
    [bundle(1, [auditId(1)]), bundle(2, [auditId(2)]), bundle(3, [auditId(3)])],
  );
  render(<DoneForYou completed={null} />);
  const first = (await screen.findByText('Closed "Deal 1" as lost')).closest(
    "li",
  );
  if (!first) throw new Error("no row for deal 1");
  // The headline carries the change alone, so the reason reads once.
  expect(within(first).getAllByText("reason 1.0")).toHaveLength(1);
  expect(within(first).getByText(en["brief.done.customerWrote"])).toBeTruthy();
  expect(within(first).getByText("We cancelled the project.")).toBeTruthy();
  // A closure says what happened under the deal's linked name, and that its
  // reason is unavailable.
  const second = screen.getByRole("link", { name: "Deal 2" }).closest("li");
  if (!second) throw new Error("no row for deal 2");
  expect(within(second).getByText(en["brief.done.markedLost"])).toBeTruthy();
  expect(within(second).queryByText(/Closed "Deal 2"/)).toBeNull();
  expect(within(second).getByText(en["brief.done.noReason"])).toBeTruthy();
  // Any other change with no reason leaves the line out.
  const third = screen.getByText('Closed "Deal 3" as lost').closest("li");
  if (!third) throw new Error("no row for deal 3");
  expect(within(third).queryByText(en["brief.done.noReason"])).toBeNull();
});

it("keeps an older server's summary whole and claims no missing reason", async () => {
  routes(
    [change(1, 0, { headline: undefined, reason: undefined })],
    [bundle(1, [auditId(1)])],
  );
  render(<DoneForYou completed={null} />);
  expect(
    await screen.findByText('Closed "Deal 1" as lost — reason 1.0'),
  ).toBeTruthy();
  expect(screen.queryByText(en["brief.done.noReason"])).toBeNull();
});

it("undoes exactly the changes a deal lists, never one that arrived since", async () => {
  const arrived = auditId(1, 9);
  const calls = routes(
    [change(1, 0), change(1, 1)],
    [bundle(1, [auditId(1, 0), auditId(1, 1), arrived])],
    () =>
      jsonResponse({
        results: [
          {
            deal_id: dealId(1),
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
        ],
      }),
  );
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  await user.click(
    await screen.findByRole("button", { name: "Undo 2 changes" }),
  );
  await screen.findByRole("status");
  const posted = writes(calls).find((c) => c.path === "/deal-corrections/undo");
  expect(posted?.body).toEqual({
    items: [
      {
        deal_id: dealId(1),
        version: 7,
        audit_log_ids: [auditId(1, 0), auditId(1, 1)],
      },
    ],
  });
});

it("lists a deal the server counts even when no loaded page holds its changes", async () => {
  routes([], [bundle(1, [auditId(1, 0), auditId(1, 1)])]);
  render(<DoneForYou completed={null} />);
  expect(await screen.findByText("1 deal waits for your review")).toBeTruthy();
  const open = screen.getByRole("link", { name: "View 2 changes" });
  expect(open.getAttribute("href")).toContain(dealId(1));
});

it("reviews all by sending only the shown changes the server still allows", async () => {
  const unseen = auditId(1, 9);
  const editedSince = auditId(1, 1);
  const calls = routes(
    [change(1, 0), change(1, 1), change(2, 0)],
    [
      {
        ...bundle(1, [auditId(1, 0), editedSince, unseen]),
        undoable_audit_log_ids: [auditId(1, 0), unseen],
      },
      bundle(2, [auditId(2, 0)]),
    ],
    () =>
      jsonResponse({
        results: [1, 2].map((n) => ({
          deal_id: dealId(n),
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        })),
      }),
  );
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  await user.click(
    await screen.findByRole("button", { name: "Review all 2 deals" }),
  );
  const drawer = await screen.findByRole("dialog");
  await user.click(
    within(drawer).getByRole("button", { name: "Undo changes on 2 deals" }),
  );
  await screen.findAllByRole("status");
  const posted = writes(calls).find((c) => c.path === "/deal-corrections/undo");
  expect(posted?.body).toEqual({
    items: [
      { deal_id: dealId(1), version: 7, audit_log_ids: [auditId(1, 0)] },
      { deal_id: dealId(2), version: 7, audit_log_ids: [auditId(2, 0)] },
    ],
  });
});

it("says the changes did not load and retries both reads when the count fails", async () => {
  let summaryReads = 0;
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts: [change(1)],
      }),
    "GET /deal-corrections/pending": () => {
      summaryReads += 1;
      return jsonResponse({ title: "unavailable" }, 503);
    },
  });
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  const retry = await screen.findByRole("button", { name: en["state.retry"] });
  // No partial list stands in for the one the server could not count.
  expect(screen.queryByText("reason 1.0")).toBeNull();
  expect(screen.queryByText(/wait.? for your review/)).toBeNull();
  const before = summaryReads;
  await user.click(retry);
  await waitFor(() => expect(summaryReads).toBeGreaterThan(before));
});

it("names each deal a review-all left partly undone, linked to it", async () => {
  routes(
    [change(1), change(2)],
    [bundle(1, [auditId(1)]), bundle(2, [auditId(2)])],
    () =>
      jsonResponse({
        results: [
          {
            deal_id: dealId(1),
            outcome: "refused",
            undone: [],
            kept: [
              {
                audit_log_id: auditId(1),
                kind: "close_lost",
                reason: "changed_since",
              },
            ],
          },
          {
            deal_id: dealId(2),
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
        ],
      }),
  );
  const user = userEvent.setup();
  render(<DoneForYou completed={null} />);
  await user.click(
    await screen.findByRole("button", { name: "Review all 2 deals" }),
  );
  const drawer = await screen.findByRole("dialog");
  await user.click(
    within(drawer).getByRole("button", { name: "Undo changes on 2 deals" }),
  );
  const list = await screen.findByRole("list", {
    name: "Deals not fully undone",
  });
  const links = within(list).getAllByRole("link");
  expect(links.map((link) => link.textContent)).toEqual(["Deal 1"]);
});

it("keeps the drawer's list and its Undo together until the reader asks for new changes", async () => {
  const receipts = [change(1), change(2)];
  const bundles = [bundle(1, [auditId(1)]), bundle(2, [auditId(2)])];
  routes(receipts, bundles);
  const user = userEvent.setup();
  const { client } = render(<DoneForYou completed={null} />);
  await user.click(
    await screen.findByRole("button", { name: "Review all 2 deals" }),
  );
  const drawer = await screen.findByRole("dialog");
  // A third change lands while the drawer is open.
  receipts.push(change(3));
  bundles.push(bundle(3, [auditId(3)]));
  await client.invalidateQueries();
  const showNew = await within(drawer).findByRole("button", {
    name: en["brief.done.showNew"],
  });
  expect(within(drawer).queryByText("reason 3.0")).toBeNull();
  await user.click(showNew);
  expect(await within(drawer).findByText("reason 3.0")).toBeTruthy();
});
