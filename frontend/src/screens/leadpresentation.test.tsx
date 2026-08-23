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
});
