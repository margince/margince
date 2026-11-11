/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
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
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
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

it("keeps the first batch's answers when a later batch fails, and sends again only what still waits", async () => {
  let posts = 0;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": (body) => {
      posts += 1;
      // The second batch fails on the wire.
      if (posts === 2) return jsonResponse({ title: "Unavailable" }, 503);
      const items = (body as { items: Item[] }).items;
      return jsonResponse({
        results: items.map((item) => ({
          deal_id: item.deal_id,
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        })),
      });
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
    await screen.findByText("Undone on 200 deals. 1 deal was not sent."),
  ).toBeTruthy();

  await user.click(screen.getByRole("button", { name: "Send again" }));
  expect(await screen.findByText("Undone on 201 deals.")).toBeTruthy();
  const sent = writes(calls)
    .filter((call) => call.path === "/deal-corrections/undo")
    .map((call) => (call.body as { items: Item[] }).items);
  expect(sent.map((items) => items.length)).toEqual([200, 1, 1]);
  expect(sent[2]).toEqual([
    { deal_id: "deal-200", version: 9, audit_log_ids: ["change-200"] },
  ]);
});

it("counts each outcome apart and never calls a partial result done", async () => {
  stubApi({
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

// The seat meFixture signs in.
const SEAT = "00000000-0000-4000-8000-000000000001";

// The change history after the server committed a batch whose answer never
// arrived: the deal's change reads reversed.
const reversedHistory = (n: number) => () =>
  jsonResponse({
    as_of: "2026-09-13T08:00:00Z",
    truncated: false,
    receipts: [
      {
        id: `change-${n}`,
        kind: "deal_close_lost",
        summary: `Closed "Deal ${n}" as lost`,
        occurred_at: "2026-09-13T02:00:00Z",
        source: "correction",
        audit_log_id: `change-${n}`,
        undo: { audit_log_id: `change-${n}`, version: 7, reversed: true },
      },
    ],
  });

it("calls a failed batch undone once the change history shows the server committed it", async () => {
  let posts = 0;
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": reversedHistory(200),
    "POST /deal-corrections/undo": (body) => {
      posts += 1;
      if (posts === 2) return jsonResponse({ title: "Unavailable" }, 503);
      const items = (body as { items: Item[] }).items;
      return jsonResponse({
        results: items.map((item) => ({
          deal_id: item.deal_id,
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        })),
      });
    },
  });
  const user = userEvent.setup();
  render(
    <BundleUndo bundles={chosen} label="Undo changes on 201 deals" remember />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Undo changes on 201 deals" }),
  );
  expect(await screen.findByText("Undone on 201 deals.")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Send again" })).toBeNull();
});

it("shows a remembered run after a reload, with the deals it could not send", async () => {
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    // Deal 2 still waits, so it stays unsent.
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
        bundles: [bundle(2)],
        truncated: false,
      }),
  });
  globalThis.localStorage.setItem(
    "margince.changes.lastBulkUndo",
    JSON.stringify({
      seat: SEAT,
      results: [
        {
          deal_id: "deal-1",
          outcome: "refused",
          undone: [],
          kept: [
            {
              audit_log_id: "change-1",
              kind: "close_lost",
              reason: "changed_since",
            },
          ],
        },
      ],
      unsent: [bundle(2)],
      names: { "deal-1": "Deal 1" },
    }),
  );
  const user = userEvent.setup();
  render(<BundleUndo bundles={[]} label="" remember />);
  expect(
    await screen.findByText("1 deal kept its changes. 1 deal was not sent."),
  ).toBeTruthy();
  expect(screen.getByRole("link", { name: "Deal 1" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Send again" })).toBeTruthy();
  await user.click(
    screen.getByRole("button", { name: en["changes.bulk.clear"] }),
  );
  expect(
    globalThis.localStorage.getItem("margince.changes.lastBulkUndo"),
  ).toBeNull();
  expect(screen.queryByText(/kept its changes/)).toBeNull();
});

it("keeps no run in the browser unless asked to remember it", async () => {
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () =>
      jsonResponse({
        results: [
          {
            deal_id: "deal-1",
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
          {
            deal_id: "deal-2",
            outcome: "undone",
            undone: ["close_lost"],
            kept: [],
          },
        ],
      }),
  });
  const user = userEvent.setup();
  render(<BundleUndo bundles={[bundle(1), bundle(2)]} label="Undo both" />);
  await user.click(await screen.findByRole("button", { name: "Undo both" }));
  expect(await screen.findByText("Undone on 2 deals.")).toBeTruthy();
  expect(
    globalThis.localStorage.getItem("margince.changes.lastBulkUndo"),
  ).toBeNull();
});

it("gives every deal of a first batch the server committed but never answered a line of its own", async () => {
  let posts = 0;
  const pendingNow = { deals: 0, bundles: [] as Bundle[] };
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    // The first 200 deals were taken back; their answer was lost on the wire.
    "POST /deal-corrections/undo": (body) => {
      posts += 1;
      if (posts === 1) {
        pendingNow.bundles = [{ ...bundle(200, 9) }];
        pendingNow.deals = 1;
        return jsonResponse({ title: "Unavailable" }, 503);
      }
      const items = (body as { items: Item[] }).items;
      pendingNow.bundles = [];
      pendingNow.deals = 0;
      return jsonResponse({
        results: items.map((item) => ({
          deal_id: item.deal_id,
          outcome: "undone",
          undone: ["close_lost"],
          kept: [],
        })),
      });
    },
    "GET /deal-corrections/pending": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        deals: pendingNow.deals,
        closed: pendingNow.deals,
        redated: 0,
        revalued: 0,
        probabilities: 0,
        moved: 0,
        opened: 0,
        revived: 0,
        base_currency: "EUR",
        bundles: pendingNow.bundles,
        truncated: false,
      }),
  });
  const user = userEvent.setup();
  render(
    <BundleUndo bundles={chosen} label="Undo changes on 201 deals" remember />,
  );
  await user.click(
    await screen.findByRole("button", { name: "Undo changes on 201 deals" }),
  );
  // Nothing loaded says the 200 were reversed; the pending read no longer
  // lists them, so none is called unsent.
  expect(
    await screen.findByText(
      "200 deals no longer wait for review. 1 deal was not sent.",
    ),
  ).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Send again" }));
  expect(
    await screen.findByText(
      "Undone on 1 deal. 200 deals no longer wait for review.",
    ),
  ).toBeTruthy();
  const sent = writes(calls)
    .filter((call) => call.path === "/deal-corrections/undo")
    .map((call) => (call.body as { items: Item[] }).items.length);
  expect(sent).toEqual([200, 1]);
});

it("keeps one seat's run from the next seat to sign in on this browser", async () => {
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
  });
  globalThis.localStorage.setItem(
    "margince.changes.lastBulkUndo",
    JSON.stringify({
      seat: "00000000-0000-4000-8000-0000000000ff",
      results: [],
      unsent: [bundle(2)],
      names: {},
    }),
  );
  render(<BundleUndo bundles={[bundle(1)]} label="Undo deal 1" remember />);
  expect(
    await screen.findByRole("button", { name: "Undo deal 1" }),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Send again" })).toBeNull();
  expect(screen.queryByText(/was not sent/)).toBeNull();
});
