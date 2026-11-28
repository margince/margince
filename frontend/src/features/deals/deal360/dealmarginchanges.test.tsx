/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import { jsonResponse, render, stubApi } from "../../../testing/brief.testkit";
import { meFixture } from "../../../testing/mefixture";
import { automaticStageReceipt } from "../../../testing/worklist.receiptreview.fixtures";
import { DealMarginceChangesCard } from "./dealmarginchanges";

type Check = components["schemas"]["DealMarginceCheck"];

const DEAL = "00000000-0000-4000-8000-000000000001";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function routes(check: Check) {
  stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    [`GET /deals/${DEAL}/margince-changes`]: () =>
      jsonResponse({ changes: [], last_check: check }),
  });
}

it("tells the owner why Margince changed nothing and which sources it read", async () => {
  routes({
    checked_at: "2026-11-04T02:00:00Z",
    next_check_at: "2026-12-02T00:00:00Z",
    explained: true,
    outcomes: [{ kind: "amount", why: "not_sure" }],
    sources: [
      { source: "mail", state: "unavailable" },
      { source: "calls", state: "private" },
      { source: "notes", state: "read" },
    ],
  });
  render(<DealMarginceChangesCard dealId={DEAL} canWrite />);
  expect(
    await screen.findByText(
      /Margince checked this deal on .* and changed nothing/,
    ),
  ).toBeTruthy();
  const sub = screen.getByText(/Amount: not sure enough/).textContent ?? "";
  expect(sub).toContain(
    "Sources: mail not read on time, calls private, left out, notes read",
  );
  expect(sub).toContain("Next check on");
  expect(sub).not.toContain(en["changes.check.ownerOnly"]);
});

it("gives a colleague when the check ran and no reason", async () => {
  routes({
    checked_at: "2026-11-04T02:00:00Z",
    explained: false,
    outcomes: [],
    sources: [],
  });
  render(<DealMarginceChangesCard dealId={DEAL} canWrite />);
  expect(
    await screen.findByText(
      "Only the deal’s owner sees why · Next check when something new arrives",
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/Sources:/)).toBeNull();
});

it("opens a change's Details with the date of the message Margince read", async () => {
  stubApi({
    "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    [`GET /deals/${DEAL}/margince-changes`]: () =>
      jsonResponse({
        changes: [
          {
            id: "00000000-0000-7000-8000-000000000101",
            kind: "deal_amount",
            summary: "Changed the amount",
            occurred_at: "2026-11-04T02:00:00Z",
            reason_kind: "inference",
            source_record: {
              type: "activity",
              id: "00000000-0000-7000-8000-000000000201",
              label: "Buyer confirmation",
            },
            source_occurred_at: "2026-11-01T09:30:00Z",
            source_occurred_precision: "instant",
          },
        ],
      }),
  });
  render(<DealMarginceChangesCard dealId={DEAL} canWrite />);
  expect(await screen.findByText(en["receipt.detail.summary"])).toBeTruthy();
  const term = screen.getByText(en["receipt.detail.sourceAt"], {
    selector: "dt",
  });
  expect(term.nextElementSibling?.textContent).toMatch(/1/);
  expect(
    screen.getByText(en["receipt.detail.reasonKind.inference"]),
  ).toBeTruthy();
});

// The Undo writes the deal. The object grant cannot see a colleague's deal
// shared read-only, so the page's own answer for the row decides.
function changeRoutes() {
  return stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    [`GET /deals/${DEAL}/margince-changes`]: () =>
      jsonResponse({ changes: [automaticStageReceipt] }),
  });
}

it("offers Undo on a deal the reader may change", async () => {
  changeRoutes();
  render(<DealMarginceChangesCard dealId={DEAL} canWrite />);
  expect(await screen.findByRole("button", { name: "Undo" })).toBeTruthy();
});

it("offers no Undo on a deal the reader may not change", async () => {
  changeRoutes();
  render(<DealMarginceChangesCard dealId={DEAL} canWrite={false} />);
  expect(await screen.findByText(/Moved PIM Rollout/)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
});
