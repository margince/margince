/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  render,
  stubApi,
  writes,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import { automaticDateReceipt } from "../../testing/worklist.receiptreview.fixtures";
import { BriefChanges } from "./brief.changes";

type Bundle = components["schemas"]["MarginceChangeBundle"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function bundle(n: number, kinds: Bundle["kinds"], audit: string[]): Bundle {
  return {
    deal_id: `00000000-0000-4000-8000-00000000000${n}`,
    deal_name: `Deal ${n}`,
    version: 7,
    audit_log_ids: audit,
    kinds,
    value_base_minor: 1_000_000 * (10 - n),
    latest_at: "2026-09-13T02:00:00Z",
  };
}

// Deal 1's bundle names the receipt the handled list also holds.
const bundles = [
  bundle(1, ["close_lost"], [automaticDateReceipt.undo?.audit_log_id ?? ""]),
  bundle(2, ["deal_close_date", "deal_amount"], ["a2", "b2"]),
  bundle(3, ["deal_close_date"], ["a3"]),
];

function routes(pending: Bundle[], undo?: (body: unknown) => Response) {
  return stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    [`GET /deals/${automaticDateReceipt.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts: [automaticDateReceipt],
      }),
    "GET /deal-corrections/pending": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        deals: pending.length,
        closed: pending.filter((b) => b.kinds.includes("close_lost")).length,
        redated: pending.filter((b) => b.kinds.includes("deal_close_date"))
          .length,
        revalued: pending.filter((b) => b.kinds.includes("deal_amount")).length,
        probabilities: 0,
        base_currency: "EUR",
        bundles: pending,
        truncated: false,
      }),
    ...(undo ? { "POST /deal-corrections/undo": undo } : {}),
  });
}

it("groups three changed deals into one row, counting deals, and shows each change once", async () => {
  routes(bundles);
  render(<BriefChanges />);
  expect(
    await screen.findByText(
      "Margince updated 3 of your deals: 1 closed, 2 re-dated, 1 re-valued",
    ),
  ).toBeTruthy();
  expect(screen.getByText("New close date · New amount")).toBeTruthy();
  // The receipt deal 1's bundle holds is in the row, not listed again.
  expect(screen.queryByText(/expected close date/i)).toBeNull();
  expect(
    screen.getByRole("link", { name: en["changes.viewAll"] }),
  ).toBeTruthy();
});

it("lists the changes one by one while fewer than three deals changed", async () => {
  routes(bundles.slice(0, 2));
  render(<BriefChanges />);
  await screen.findByRole("heading", { name: en["brief.changes.title"] });
  expect(screen.queryByText(/Margince updated/)).toBeNull();
});

it("undoes one deal's bundle and names the field it kept", async () => {
  const calls = routes(bundles, () =>
    jsonResponse({
      results: [
        {
          deal_id: bundles[1]?.deal_id,
          outcome: "partly_undone",
          undone: ["deal_close_date"],
          kept: [
            {
              audit_log_id: "b2",
              kind: "deal_amount",
              reason: "changed_since",
            },
          ],
        },
      ],
    }),
  );
  const user = userEvent.setup();
  render(<BriefChanges />);
  const row = (await screen.findByText("New close date · New amount")).closest(
    "li",
  );
  if (!row) throw new Error("no row for deal 2");
  await user.click(
    await within(row).findByRole("button", { name: en["common.undo"] }),
  );
  expect(
    await screen.findByText(
      "Undone, except the amount, which was changed since Margince set it.",
    ),
  ).toBeTruthy();
  const posted = writes(calls).find((c) => c.path === "/deal-corrections/undo");
  expect(posted?.body).toEqual({
    items: [
      { deal_id: bundles[1]?.deal_id, version: 7, audit_log_ids: ["a2", "b2"] },
    ],
  });
});

it("undoes every deal at once and says how many", async () => {
  const calls = routes(bundles, (body) => {
    const items = (body as { items: { deal_id: string }[] }).items;
    return jsonResponse({
      results: items.map((item) => ({
        deal_id: item.deal_id,
        outcome: "undone",
        undone: ["close_lost"],
        kept: [],
      })),
    });
  });
  const user = userEvent.setup();
  render(<BriefChanges />);
  await user.click(
    await screen.findByRole("button", { name: en["changes.undoAll"] }),
  );
  expect(await screen.findByText("Undone on 3 deals.")).toBeTruthy();
  await waitFor(() =>
    expect(
      writes(calls).filter((c) => c.path === "/deal-corrections/undo"),
    ).toHaveLength(1),
  );
});
