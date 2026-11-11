/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { keepUndoManifest } from "../lib/marginchanges.runs";
import { isRecord } from "../lib/problembody";
import { forgetSeat } from "../lib/storage";
import {
  jsonResponse,
  render,
  stubApi,
  writes,
} from "../testing/brief.testkit";
import { meFixture } from "../testing/mefixture";
import { BundleUndo } from "./marginchanges";

type Bundle = components["schemas"]["MarginceChangeBundle"];
type Item = { deal_id: string; version: number; audit_log_ids: string[] };

afterEach(() => {
  cleanup();
  forgetSeat();
  vi.unstubAllGlobals();
});

const bundle = (n: number, version = 7): Bundle => ({
  deal_id: `deal-${n}`,
  deal_name: `Deal ${n}`,
  version,
  audit_log_ids: [`change-${n}`],
  kinds: ["close_lost"],
  latest_at: "2026-09-13T02:00:00Z",
});

// One more deal than a batch holds, so the Undo takes two requests.
const chosen = Array.from({ length: 201 }, (_, i) => bundle(i));

it("refuses to start a bulk Undo whose recovery request cannot be saved", async () => {
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
  });
  vi.stubGlobal("localStorage", undefined);
  const user = userEvent.setup();
  render(<BundleUndo bundles={chosen.slice(0, 2)} label="Undo two deals" />);
  await user.click(
    await screen.findByRole("button", { name: "Undo two deals" }),
  );
  expect(
    await screen.findByText(
      "Bulk Undo could not be saved. Allow site storage and try again.",
    ),
  ).toBeTruthy();
  expect(calls.some((call) => call.path === "/deal-corrections/undo")).toBe(
    false,
  );
});

it("keeps the first batch's saved answers and resumes the failed batch with its original items", async () => {
  let posts = 0;
  const saved: components["schemas"]["MarginceUndoRun"][] = [];
  const calls = stubApi({
    "POST /deal-corrections/undo/results": () => jsonResponse({ runs: saved }),
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": (body) => {
      posts += 1;
      // The second batch fails on the wire.
      if (posts === 2) return jsonResponse({ title: "Unavailable" }, 503);
      const items = (body as { items: Item[] }).items;
      const results: components["schemas"]["UndoMarginceChangeResult"][] =
        items.map((item) => ({
          deal_id: item.deal_id,
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        }));
      if (!isRecord(body) || typeof body.run_id !== "string")
        throw new Error("missing run id");
      const runId = body.run_id;
      saved.push({
        run_id: runId,
        items,
        results,
        complete: true,
        created_at: "2026-09-13T08:00:00Z",
      });
      return jsonResponse({ results, run_id: runId, complete: true });
    },
    // Read again before the retry: the failed deal still waits, at a newer
    // version, and has a change the reader never chose.
    "GET /deal-corrections/pending": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        deals: 1,
        closed: 1,
        redated: 0,
        revalued: 0,
        probabilities: 0,
        moved: 0,
        opened: 0,
        revived: 0,
        base_currency: "EUR",
        bundles: [
          { ...bundle(200, 9), audit_log_ids: ["change-200", "arrived-200"] },
        ],
        truncated: false,
      }),
  });
  const user = userEvent.setup();
  render(<BundleUndo bundles={chosen} label="Undo changes on 201 deals" />);
  await user.click(
    await screen.findByRole("button", { name: "Undo changes on 201 deals" }),
  );
  expect(
    await screen.findByText("Undone on 200 deals. No result yet for 1 deal."),
  ).toBeTruthy();

  const failed = calls.filter(
    (call) => call.path === "/deal-corrections/undo",
  )[1]?.body;
  if (!isRecord(failed) || typeof failed.run_id !== "string")
    throw new Error("missing failed run");
  keepUndoManifest([
    { deal_id: "another-deal", version: 1, audit_log_ids: ["another-change"] },
  ]);

  await user.click(screen.getByRole("button", { name: "Send again" }));
  expect(await screen.findByText("Undone on 201 deals.")).toBeTruthy();
  const sent = writes(calls)
    .filter((call) => call.path === "/deal-corrections/undo")
    .map((call) => (call.body as { items: Item[] }).items);
  expect(sent.map((items) => items.length)).toEqual([200, 1, 1]);
  expect(sent[2]).toEqual([
    { deal_id: "deal-200", version: 7, audit_log_ids: ["change-200"] },
  ]);
  expect(
    calls.filter((call) => call.path === "/deal-corrections/undo")[2]?.body,
  ).toMatchObject({ run_id: failed.run_id });
});

it("counts each outcome apart and never calls a partial result done", async () => {
  stubApi({
    "POST /deal-corrections/undo/results": () => jsonResponse({ runs: [] }),
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () =>
      jsonResponse({
        results: [
          {
            deal_id: "deal-0",
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
          {
            deal_id: "deal-1",
            outcome: "partly_undone",
            undone: ["close_lost"],
            kept: [
              {
                audit_log_id: "x",
                kind: "deal_amount",
                reason: "changed_since",
              },
            ],
          },
          {
            deal_id: "deal-2",
            outcome: "refused",
            undone: [],
            kept: [
              {
                audit_log_id: "y",
                kind: "close_lost",
                reason: "changed_since",
              },
            ],
          },
          {
            deal_id: "deal-3",
            outcome: "refused",
            undone: [],
            kept: [
              {
                audit_log_id: "z",
                kind: "deal_open",
                reason: "needs_confirmation",
                team_notes: 1,
                team_tasks: 0,
                team_work: "w",
              },
            ],
          },
        ],
      }),
  });
  const user = userEvent.setup();
  render(
    <BundleUndo bundles={chosen.slice(0, 4)} label="Undo changes on 4 deals" />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Undo changes on 4 deals" }),
  );
  expect(
    await screen.findByText(
      "Undone on 1 deal. 1 deal was partly undone. 1 deal kept its changes. 1 deal needs your confirmation. Undo it on its own row.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Send again" })).toBeNull();
});

function pendingWith(bundles: Bundle[], truncated = false) {
  return jsonResponse({
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
    truncated,
  });
}

it("confirms a retried deal at the version the retry read, not the first press's", async () => {
  let posts = 0;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () => {
      posts += 1;
      if (posts === 1) return jsonResponse({ title: "Unavailable" }, 503);
      return jsonResponse({
        results: [
          posts === 2
            ? {
                deal_id: "deal-0",
                outcome: "refused",
                undone: [],
                kept: [
                  {
                    audit_log_id: "change-0",
                    kind: "deal_open",
                    reason: "needs_confirmation",
                    team_notes: 1,
                    team_tasks: 0,
                    team_work: "w9",
                  },
                ],
              }
            : {
                deal_id: "deal-0",
                outcome: "undone",
                undone: ["deal_open"],
                kept: [],
              },
        ],
      });
    },
    "GET /deal-corrections/pending": () => pendingWith([bundle(0, 9)]),
  });
  const user = userEvent.setup();
  render(<BundleUndo bundles={[bundle(0)]} label="Undo" />);
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  await user.click(await screen.findByRole("button", { name: "Send again" }));
  await screen.findByText(/Archive it anyway\?/);
  await user.click(screen.getByRole("button", { name: "Undo" }));
  await screen.findByText(en["changes.undo.archived"]);
  const sent = writes(calls)
    .filter((call) => call.path === "/deal-corrections/undo")
    .map(
      (call) =>
        (call.body as { items: (Item & { archive_confirmed_for?: string })[] })
          .items[0],
    );
  expect(sent.map((item) => item?.version)).toEqual([7, 9, 9]);
  expect(sent[2]?.archive_confirmed_for).toBe("w9");
});

it("sends a deal again when a cut-short read leaves it out, rather than calling it settled", async () => {
  let posts = 0;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () => {
      posts += 1;
      if (posts === 1) return jsonResponse({ title: "Unavailable" }, 503);
      return jsonResponse({
        results: [
          {
            deal_id: "deal-0",
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
        ],
      });
    },
    // The read stopped at its bound before it reached deal 0.
    "GET /deal-corrections/pending": () => pendingWith([bundle(5)], true),
  });
  const user = userEvent.setup();
  render(<BundleUndo bundles={[bundle(0)]} label="Undo" />);
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  await user.click(await screen.findByRole("button", { name: "Send again" }));
  await screen.findByRole("status");
  const sent = writes(calls).filter(
    (call) => call.path === "/deal-corrections/undo",
  );
  expect(sent).toHaveLength(2);
  expect((sent[1]?.body as { items: Item[] } | undefined)?.items).toEqual([
    { deal_id: "deal-0", version: 7, audit_log_ids: ["change-0"] },
  ]);
});
