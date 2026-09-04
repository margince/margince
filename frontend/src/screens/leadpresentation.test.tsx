/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { LeadBoard, scoreTone } from "./leadpresentation";

type Lead = components["schemas"]["Lead"];

function renderBoard(ui: ReactNode) {
  return renderBoardWithClient(ui).rendered;
}

// The same render, recording what the board invalidated. A drag is a WRITE,
// and what it makes stale is not visible on the board itself — which is
// exactly how the defect below survived: the board refetched, looked right,
// and the detail page behind it did not.
function renderBoardWithClient(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const invalidated: unknown[] = [];
  const real = client.invalidateQueries.bind(client);
  client.invalidateQueries = ((filters?: { queryKey?: unknown }) => {
    invalidated.push(filters?.queryKey);
    return real(filters as Parameters<typeof real>[0]);
  }) as typeof client.invalidateQueries;
  const rendered = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
  return { rendered, invalidated };
}

/** Drops the card for `leadId` on the column whose stage is `stage`. */
function dropOn(stage: string, leadId: string) {
  const column = document.querySelector(`.board-col[data-stage="${stage}"]`);
  if (column === null) {
    throw new Error(`no board column for stage ${stage}`);
  }
  fireEvent.drop(column, {
    dataTransfer: { getData: () => leadId },
  });
}

const lead: Lead = {
  id: "00000000-0000-0000-0000-000000000001",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  title: "VP Sales",
  status: "new",
  score: 72,
  score_reason: "manual:employees",
  sla_state: "breached",
  source: "webform",
  next_task_subject: "Call about the pilot",
  open_task_count: 1,
  captured_by: "human:user-1",
  version: 1,
  created_at: "2026-08-18T08:00:00Z",
  updated_at: "2026-08-18T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("lead work-board presentation", () => {
  it("uses neutral styling for a low score", () => {
    expect(scoreTone(0)).toBeUndefined();
  });

  it("uses lead-specific counts and shows work context on every card", () => {
    renderBoard(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    expect(screen.getByText("1 leads")).toBeTruthy();
    expect(screen.queryByText(/deals/i)).toBeNull();
    expect(screen.getByText("Overdue")).toBeTruthy();
    expect(screen.getByText("Web form")).toBeTruthy();
    expect(screen.getByText(/Call about the pilot · 1 open task/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /next page/i })).toBeNull();
  });

  // Dragging a card WRITES the lead. The board reads `["leads", query]`; the
  // detail page reads the sibling `["lead", id]`, and prefix invalidation does
  // not walk sideways — so naming only the list left an open detail page
  // showing the status the reader had just dragged away from.
  it("invalidates the MOVED lead, not only the list it was dragged on", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ ...lead, status: "contacted" }), {
            status: 200,
            headers: { "content-type": "application/json", etag: '"2"' },
          }),
      ),
    );
    const { invalidated } = renderBoardWithClient(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    dropOn("contacted", lead.id);

    await waitFor(() => expect(invalidated).toContainEqual(["lead", lead.id]));
    expect(invalidated).toContainEqual(["leads"]);
    expect(invalidated).toContainEqual(["record-history", "lead", lead.id]);
  });

  // The refused arm owes the same. A move the server turned down means the row
  // on screen may no longer be what the server holds, which is precisely when
  // a stale detail page misleads — and the original code did name the list
  // here, so the omission was of the detail page specifically, not of
  // invalidation as an idea.
  it("invalidates the moved lead when the move is REFUSED too", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              code: "version_conflict",
              detail: "Somebody else moved this lead.",
            }),
            { status: 409, headers: { "content-type": "application/json" } },
          ),
      ),
    );
    const { invalidated } = renderBoardWithClient(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    dropOn("contacted", lead.id);

    await waitFor(() => expect(invalidated).toContainEqual(["lead", lead.id]));
    expect(await screen.findByText(/moved this lead/)).toBeTruthy();
  });
  // The two terminal columns exist, are folded, and state a figure the board's
  // own page cannot see.
  //
  // A promoted or disqualified lead is ARCHIVED, and `rows` never contains
  // one — so a column counting its cards would read 0 however many leads had
  // passed through. The figure comes from the leads-by-status report, which is
  // the only lead read that counts archived rows.
  it("folds the terminal columns and counts them from the report, not the page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonOk({
          rows: [
            { status: "promoted", leads: 12 },
            { status: "disqualified", leads: 7 },
          ],
        }),
      ),
    );
    renderBoard(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    const qualified = await waitFor(() => {
      const column = document.querySelector(
        '.board-col[data-stage="promoted"]',
      );
      if (column === null) throw new Error("no Qualified column");
      if (!column.textContent?.includes("12")) {
        throw new Error(`Qualified reads ${column.textContent}`);
      }
      return column;
    });
    // Folded, and holding no cards: `rows` has none to give it.
    expect(qualified.classList.contains("board-col-collapsed")).toBe(true);
    expect(qualified.querySelector(".deal-card")).toBeNull();

    const disqualified = document.querySelector(
      '.board-col[data-stage="disqualified"]',
    );
    expect(disqualified?.textContent).toContain("7");
    expect(disqualified?.classList.contains("board-col-collapsed")).toBe(true);
  });

  // Dropping on a terminal column opens its DIALOG and sends no PATCH.
  //
  // Neither transition is a status change: qualifying promotes the lead into a
  // person and maybe a deal, disqualifying records a reason. The server refuses
  // a bare status PATCH into either, so a board that sent one would put a
  // refusal in front of a reader who did the ordinary thing.
  it("opens the qualify dialog on a drop, instead of patching the status", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/reports/")) {
          return jsonOk({ rows: [{ status: "promoted", leads: 3 }] });
        }
        return jsonOk({ data: [] });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    renderBoard(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    dropOn("promoted", lead.id);

    // The dialog is up.
    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
    // And nothing was PATCHed. A status PATCH here is the defect: the board
    // would be asking the server for a transition it refuses by design.
    for (const call of fetchMock.mock.calls) {
      const init = call[1] as RequestInit | undefined;
      expect(String(init?.method ?? "GET").toUpperCase()).not.toBe("PATCH");
    }
  });

  // A collapsed column is still a DROP TARGET, which is the whole reason it
  // keeps its height. Folded to a strip it would still have to take a card.
  it("keeps the folded column droppable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonOk({ rows: [{ status: "disqualified", leads: 1 }] }),
      ),
    );
    renderBoard(
      <LeadBoard
        rows={[lead]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    await waitFor(() => {
      const column = document.querySelector(
        '.board-col[data-stage="disqualified"]',
      );
      if (column === null) throw new Error("no Disqualified column");
      if (!column.classList.contains("board-col-collapsed")) {
        throw new Error("Disqualified is not folded");
      }
    });
    dropOn("disqualified", lead.id);
    await waitFor(() => expect(screen.getByRole("dialog")).toBeTruthy());
  });

  // An archived lead does not drag, and is not acted on if one arrives anyway.
  //
  // Every destination refuses it: UpdateLead reads live rows only, so a drop on
  // an open stage would 409, and the other terminal dialog's own mutation
  // rejects it too. Offering the gesture and then failing it is worse than not
  // offering it — so the card carries no drag handlers AND the drop decision
  // refuses a terminal source, because a drop carries an id through a
  // dataTransfer string that no card had to originate.
  it("neither drags a terminal lead nor acts on one that is dropped", async () => {
    const terminal: Lead = { ...lead, status: "disqualified" };
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, _init?: RequestInit) => {
        const url = String(input);
        if (url.includes("/reports/")) {
          return jsonOk({ rows: [{ status: "disqualified", leads: 1 }] });
        }
        return jsonOk({
          data: [terminal],
          page: { has_more: false, next_cursor: null },
        });
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    renderBoard(
      <LeadBoard
        rows={[]}
        onMoved={() => undefined}
        hasMore={false}
        loadMore={() => undefined}
      />,
    );

    const head = await waitFor(() => {
      const found = document.querySelector(
        '.board-col[data-stage="disqualified"] .board-col-head',
      );
      if (found === null) throw new Error("no Disqualified head");
      return found;
    });
    fireEvent.click(head);

    const card = await waitFor(() => {
      const found = document.querySelector(
        `.deal-card[data-lead="${terminal.id}"]`,
      );
      if (found === null) throw new Error("the opened column drew no card");
      return found;
    });
    expect(card.getAttribute("draggable")).not.toBe("true");

    // And a drop naming it anyway moves nothing: no PATCH, no dialog.
    dropOn("contacted", terminal.id);
    expect(screen.queryByRole("dialog")).toBeNull();
    for (const call of fetchMock.mock.calls) {
      const init = call[1] as RequestInit | undefined;
      expect(String(init?.method ?? "GET").toUpperCase()).not.toBe("PATCH");
    }
  });
});

// jsonOk is one JSON 200, spelled once — every stub in the new tests answers
// the same shape and a second spelling is a second thing to keep in step.
function jsonOk(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}
