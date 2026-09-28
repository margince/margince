/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { DealBulkBar } from "./dealbulk";

// Where the roster caveat sits in the bulk bar, and why it is not a free choice.
//
// The bar is one wrapping flex row of controls, so a sentence rendered between
// the owner picker and the button that assigns it becomes a flex item BETWEEN a
// control and its verb — and the point the row breaks on a narrow viewport,
// which is where the German string puts it first. A caveat cannot be allowed to
// separate the two halves of one action, so it goes last, and the picker points
// at it by id for a reader who never sees the layout at all.

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

// Typed, not asserted: a fixture cast into the contract type can drop a required
// field and still compile, so the test would go on passing after the wire shape
// moved under it.
const DEAL: Deal = {
  id: "d-1",
  name: "Brandt renewal",
  pipeline_id: "p-1",
  stage_id: "s-1",
  status: "open",
  source: "manual",
  captured_by: "human:u-1",
  version: 1,
  created_at: "2026-07-01T08:00:00Z",
  updated_at: "2026-07-01T08:00:00Z",
};

const STAGE: Stage = {
  id: "s-1",
  pipeline_id: "p-1",
  name: "Qualified",
  position: 1,
  semantic: "open",
  win_probability: 40,
};

// A stage the fixture deal is NOT in: the bar leaves a row alone when the move
// would not move it, so a one-stage fixture proves nothing about the move verb.
const OTHER_STAGE: Stage = {
  ...STAGE,
  id: "s-2",
  name: "Proposal",
  position: 2,
};

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

/**
 * A roster that never stops offering another page, which is the only way this
 * caveat appears at all: the walk stops at its page budget and reports the list
 * as part of one.
 */
function stubTruncatedRoster() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const cursor = url.searchParams.get("cursor");
      const index = cursor ? Number(cursor) : 0;
      return json({
        data: [{ id: `u-${index}`, display_name: `Member ${index}` }],
        page: { next_cursor: String(index + 1), has_more: true },
      });
    }),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the bulk bar's roster caveat", () => {
  it("comes after every verb, never between the owner picker and the button that applies it", async () => {
    stubTruncatedRoster();
    render(
      <DealBulkBar
        deals={[DEAL]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    const note = await screen.findByText(en["state.partial"]);
    for (const label of [
      en["bulk.assign"],
      en["deals.bulkMove"],
      en["bulk.archive"],
    ]) {
      const verb = screen.getByRole("button", { name: label });
      // DOCUMENT_POSITION_FOLLOWING reads "the note comes after this button in
      // document order", which is the order the flex row lays out and the order
      // a screen reader walks.
      expect(
        verb.compareDocumentPosition(note) & Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    }
  });

  it("stays attached to the picker it is about, through the picker's description", async () => {
    stubTruncatedRoster();
    render(
      <DealBulkBar
        deals={[DEAL]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    const note = await screen.findByText(en["state.partial"]);
    const picker = screen.getByRole("combobox", {
      name: en["bulk.owner"],
    });

    expect(note.id).not.toBe("");
    expect(picker).toHaveAttribute("aria-describedby", note.id);
  });

  it("does not announce itself as news inside the bar's live region", async () => {
    stubTruncatedRoster();
    render(
      <DealBulkBar
        deals={[DEAL]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    // The bar is `aria-live="polite"`, so the caveat arriving when the walk
    // finishes would be read out mid-interaction — a fact about a list the
    // reader was not asking about, interrupting the one they were.
    expect(await screen.findByText(en["state.partial"])).toHaveAttribute(
      "aria-live",
      "off",
    );
  });
});

// Moving a stage fans out one advance per deal, each with the row's own
// version guard: a guard pinned to the wrong revision refuses a write nobody
// raced, and one missing lets a stale move overwrite a concurrent edit.
describe("the bulk bar's stage move", () => {
  it("pins the row's version when moving a stage", async () => {
    const sent: Request[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : null;
        if (request && request.method !== "GET") {
          sent.push(request);
          return json(DEAL);
        }
        return json({
          data: [{ id: "u-1", display_name: "Member" }],
          page: { has_more: false },
        });
      }),
    );
    const user = userEvent.setup();
    render(
      <DealBulkBar
        deals={[DEAL]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    // The deal is in s-1 already, and the bar skips a row that would not move —
    // so the fixture needs a SECOND stage for the move to be a move at all.
    await user.click(
      screen.getByRole("combobox", { name: en["deals.bulkStage"] }),
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: OTHER_STAGE.name,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["deals.bulkMove"] }),
    );

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].method).toBe("POST");
    expect(new URL(sent[0].url, "https://test.local").pathname).toBe(
      "/v1/deals/d-1/advance",
    );
    expect(sent[0].headers.get("If-Match")).toBe(String(DEAL.version));
  });
});

// Owner and archive are ONE preview and ONE change on the server, whatever the
// selection size, each deal carrying its own version. A fan-out of one PATCH or
// DELETE per deal from the browser is exactly what these replace.
describe("the bulk bar's owner and archive verbs", () => {
  const SECOND: Deal = {
    ...DEAL,
    id: "d-2",
    name: "Ott expansion",
    version: 5,
  };

  function stubBulk() {
    const writes: { path: string; body: unknown }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        const path = new URL(input.url, "https://test.local").pathname;
        if (input.method !== "GET") {
          writes.push({ path, body: await input.clone().json() });
        }
        if (path === "/v1/bulk/preview") {
          return json({
            record_type: "deal",
            verb: "archive",
            count: 2,
            affected: ["d-1", "d-2"],
            excluded: [],
            sample: [],
            requires_confirmation: false,
          });
        }
        if (path === "/v1/bulk/execute") {
          return json({ batch_id: "b-1", changed: 2, skipped: [] });
        }
        return json({
          data: [{ id: "u-1", display_name: "Member" }],
          page: { has_more: false },
        });
      }),
    );
    return writes;
  }

  const ITEMS = [
    { id: "d-1", version: DEAL.version },
    { id: "d-2", version: 5 },
  ];

  it("assigns an owner through one preview and one execute", async () => {
    const writes = stubBulk();
    const onDone = vi.fn();
    const user = userEvent.setup();
    render(
      <DealBulkBar
        deals={[DEAL, SECOND]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={onDone}
      />,
    );

    await user.click(
      await screen.findByRole("combobox", { name: en["bulk.owner"] }),
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Member",
      }),
    );
    await user.click(screen.getByRole("button", { name: en["bulk.assign"] }));
    const dialog = await screen.findByRole("dialog");
    await user.click(
      await within(dialog).findByRole("button", {
        name: en["bulk.confirmReassign"],
      }),
    );

    await waitFor(() => expect(onDone).toHaveBeenCalledWith([]));
    expect(writes).toEqual([
      {
        path: "/v1/bulk/preview",
        body: {
          record_type: "deal",
          verb: "reassign_owner",
          items: ITEMS,
          owner_id: "u-1",
        },
      },
      {
        path: "/v1/bulk/execute",
        body: {
          record_type: "deal",
          verb: "reassign_owner",
          items: ITEMS,
          owner_id: "u-1",
        },
      },
    ]);
  });

  it("archives through one preview and one execute", async () => {
    const writes = stubBulk();
    const user = userEvent.setup();
    render(
      <DealBulkBar
        deals={[DEAL, SECOND]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    await user.click(screen.getByRole("button", { name: en["bulk.archive"] }));
    const dialog = await screen.findByRole("dialog");
    const confirm = await within(dialog).findByRole("button", {
      name: en["bulk.confirmArchive"].replace("{unit}", en["unit.deals"]),
    });
    // Archiving many deals is the most destructive thing this bar does, so
    // nothing but the preview has reached the server before the confirm.
    expect(writes.map((write) => write.path)).toEqual(["/v1/bulk/preview"]);
    await user.click(confirm);

    await waitFor(() => expect(writes).toHaveLength(2));
    expect(writes.map((write) => write.path)).toEqual([
      "/v1/bulk/preview",
      "/v1/bulk/execute",
    ]);
    expect(writes[1].body).toEqual({
      record_type: "deal",
      verb: "archive",
      items: ITEMS,
    });
  });
});
