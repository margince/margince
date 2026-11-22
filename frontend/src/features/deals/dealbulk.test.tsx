/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { seedWriter } from "../../testing/mefixture";
import { DealBulkBar } from "./dealbulk";
import { render as renderWithToasts } from "./deals.testkit";

// The stage move asks the deal update grant, so the bar is drawn for a writer.
function renderBar(ui: ReactNode) {
  return renderWithToasts(
    ui,
    seedWriter(
      new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    ),
  );
}

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
  seedWriter(client);
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

async function confirmMove(user: ReturnType<typeof userEvent.setup>) {
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: en["deals.bulkMove"] }),
  );
}

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
    await confirmMove(user);

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].method).toBe("POST");
    expect(new URL(sent[0].url, "https://test.local").pathname).toBe(
      "/v1/deals/d-1/advance",
    );
    expect(sent[0].headers.get("If-Match")).toBe(String(DEAL.version));
  });
});

describe("the bulk bar's stage move result", () => {
  const SECOND: Deal = { ...DEAL, id: "d-2", name: "Ott expansion" };
  const THIRD: Deal = { ...DEAL, id: "d-3", name: "Kern pilot" };

  function stubAdvances() {
    const advanced: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        const path = new URL(input.url, "https://test.local").pathname;
        if (input.method === "POST") {
          advanced.push(path);
          return json(DEAL);
        }
        return json({ data: [], page: { has_more: false } });
      }),
    );
    return advanced;
  }

  async function moveTo(
    user: ReturnType<typeof userEvent.setup>,
    stage: Stage,
  ) {
    await user.click(
      screen.getByRole("combobox", { name: en["deals.bulkStage"] }),
    );
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: stage.name,
      }),
    );
    await user.click(
      screen.getByRole("button", { name: en["deals.bulkMove"] }),
    );
  }

  it("says how many moved and how many were already there", async () => {
    const advanced = stubAdvances();
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[DEAL, SECOND, { ...THIRD, stage_id: OTHER_STAGE.id }]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    await moveTo(user, OTHER_STAGE);
    await confirmMove(user);

    expect(
      await screen.findByText(
        "2 deals moved to Proposal. 1 was already in Proposal.",
      ),
    ).toBeTruthy();
    expect(advanced).toEqual([
      "/v1/deals/d-1/advance",
      "/v1/deals/d-2/advance",
    ]);
  });

  it("sends nothing and keeps the selection when every deal is already there", async () => {
    const advanced = stubAdvances();
    const onDone = vi.fn();
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[DEAL, SECOND, THIRD]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={onDone}
      />,
    );

    await moveTo(user, STAGE);

    expect(
      await screen.findByText("All 3 selected deals are already in Qualified."),
    ).toBeTruthy();
    expect(advanced).toEqual([]);
    expect(onDone).not.toHaveBeenCalled();
  });
});

describe("the bulk bar's stage move confirmation", () => {
  const rows = (n: number, stageId = STAGE.id): Deal[] =>
    Array.from({ length: n }, (_, i) => ({
      ...DEAL,
      id: `d-${i + 1}`,
      name: `Deal ${i + 1}`,
      stage_id: stageId,
    }));

  function stubWrites() {
    const advanced: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.method === "POST") {
          advanced.push(new URL(input.url, "https://test.local").pathname);
          return json(DEAL);
        }
        return json({ data: [], page: { has_more: false } });
      }),
    );
    return advanced;
  }

  async function press(user: ReturnType<typeof userEvent.setup>) {
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
    return screen.findByRole("dialog");
  }

  it("shows what a move of 3 deals does and writes nothing until Confirm", async () => {
    const advanced = stubWrites();
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={rows(3)}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    const dialog = await press(user);

    expect(within(dialog).getByText("Move 3 deals to Proposal?")).toBeTruthy();
    expect(
      within(dialog).getByText("3 deals will move to Proposal."),
    ).toBeTruthy();
    expect(within(dialog).queryByText(en["bulk.largeTitle"])).toBeNull();
    expect(advanced).toEqual([]);
  });

  it("warns Large change for 11 deals but not for 10", async () => {
    stubWrites();
    const user = userEvent.setup();
    const { unmount } = renderBar(
      <DealBulkBar
        deals={rows(11)}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );
    expect(
      within(await press(user)).getByText(en["bulk.largeTitle"]),
    ).toBeTruthy();
    unmount();
    cleanup();

    renderBar(
      <DealBulkBar
        deals={rows(10)}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );
    expect(
      within(await press(user)).queryByText(en["bulk.largeTitle"]),
    ).toBeNull();
  });

  it("counts deals already in the stage and leaves them out of the warning", async () => {
    stubWrites();
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[
          ...rows(10),
          ...rows(4, OTHER_STAGE.id).map((d, i) => ({ ...d, id: `x-${i}` })),
        ]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    const dialog = await press(user);

    expect(
      within(dialog).getByText(
        "10 deals will move to Proposal. 4 were already in Proposal.",
      ),
    ).toBeTruthy();
    expect(within(dialog).queryByText(en["bulk.largeTitle"])).toBeNull();
  });

  it("moves nothing when the reader cancels", async () => {
    const advanced = stubWrites();
    const onDone = vi.fn();
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={rows(3)}
        stages={[STAGE, OTHER_STAGE]}
        onDone={onDone}
      />,
    );

    const dialog = await press(user);
    await user.click(
      within(dialog).getByRole("button", { name: en["create.cancel"] }),
    );

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(advanced).toEqual([]);
    expect(onDone).not.toHaveBeenCalled();
  });
});

describe("the bulk bar's stage move edge cases", () => {
  const SECOND: Deal = { ...DEAL, id: "d-2", name: "Ott expansion" };
  const THIRD: Deal = { ...DEAL, id: "d-3", name: "Kern pilot" };

  function problem(status: number) {
    return new Response(
      JSON.stringify({ title: "x", status, code: "conflict", detail: "moved" }),
      {
        status,
        headers: { "content-type": "application/problem+json" },
      },
    );
  }

  function stubAdvance(refuse: (path: string) => Response | null) {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.method === "POST") {
          const path = new URL(input.url, "https://test.local").pathname;
          return refuse(path) ?? json(DEAL);
        }
        return json({ data: [], page: { has_more: false } });
      }),
    );
  }

  async function pressMove(user: ReturnType<typeof userEvent.setup>) {
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
  }

  it("closes the dialog when the selection changes under it, so no empty stage is sent", async () => {
    const sent: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request) => {
        if (input.method === "POST") {
          sent.push(await input.clone().text());
        }
        return json({ data: [], page: { has_more: false } });
      }),
    );
    function Harness() {
      const [rows, setRows] = useState([DEAL, SECOND]);
      return (
        <>
          <button type="button" onClick={() => setRows([THIRD])}>
            swap selection
          </button>
          <DealBulkBar
            deals={rows}
            stages={[STAGE, OTHER_STAGE]}
            onDone={() => {}}
          />
        </>
      );
    }
    const user = userEvent.setup();
    renderBar(<Harness />);

    await pressMove(user);
    expect(await screen.findByRole("dialog")).toBeTruthy();
    // The dialog is modal, so the selection changes from the outside.
    act(() => {
      screen.getByText("swap selection").click();
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(sent).toEqual([]);
  });

  it("moves the rest and names the deal that answered 409", async () => {
    stubAdvance((path) =>
      path === "/v1/deals/d-2/advance" ? problem(409) : null,
    );
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[DEAL, SECOND, THIRD]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    await pressMove(user);
    await confirmMove(user);

    expect(await screen.findByText("2 deals moved to Proposal.")).toBeTruthy();
    expect(
      await screen.findByText(/1 not applied:.*Ott expansion/),
    ).toBeTruthy();
  });

  it("says nothing moved when every deal is refused", async () => {
    stubAdvance(() => problem(409));
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[DEAL, SECOND]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    await pressMove(user);
    await confirmMove(user);

    expect(await screen.findByText(/2 not applied:/)).toBeTruthy();
    expect(screen.queryByText(/moved to Proposal/)).toBeNull();
  });

  it("uses the singular wording for one deal", async () => {
    stubAdvance(() => null);
    const user = userEvent.setup();
    renderBar(
      <DealBulkBar
        deals={[DEAL]}
        stages={[STAGE, OTHER_STAGE]}
        onDone={() => {}}
      />,
    );

    await pressMove(user);
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Move 1 deal to Proposal?")).toBeTruthy();
    expect(
      within(dialog).getByText("1 deal will move to Proposal."),
    ).toBeTruthy();
    await confirmMove(user);
    expect(await screen.findByText("1 deal moved to Proposal.")).toBeTruthy();
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
