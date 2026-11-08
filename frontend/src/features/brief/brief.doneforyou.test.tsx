/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
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
      change(2, 0, { reason: undefined }),
    ],
    [bundle(1, [auditId(1)]), bundle(2, [auditId(2)])],
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
  const second = screen.getByText('Closed "Deal 2" as lost').closest("li");
  if (!second) throw new Error("no row for deal 2");
  expect(within(second).getByText(en["brief.done.noReason"])).toBeTruthy();
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
