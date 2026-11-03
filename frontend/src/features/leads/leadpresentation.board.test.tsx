/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import type { ListQuery } from "../../components/listquery";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { LEAD_LIST_KEY } from "../../lib/leadkeys";
import { UNNARROWED_LIST_QUERY } from "../../testing/listquery.fixture";
import { LeadBoard } from "./leadpresentation.board";

type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "00000000-0000-0000-0000-000000000001",
  full_name: "Jonas Petersen",
  status: "new",
  score: 72,
  source: "webform",
  captured_by: "human:user-1",
  version: 1,
  created_at: "2026-08-18T08:00:00Z",
  updated_at: "2026-08-18T08:00:00Z",
};

function mine(filters: Record<string, string> = {}): ListQuery {
  return {
    ...UNNARROWED_LIST_QUERY,
    filters: { owner_id: "u-me", ...filters },
  };
}

type Served = { method: string; url: URL; body: unknown };

// Every request the board sends, and the answer each gets.
function serve(answer: (request: Served) => Response): Served[] {
  const served: Served[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const text = await request.clone().text();
      const sent = {
        method: request.method,
        url: new URL(request.url),
        body: text ? JSON.parse(text) : undefined,
      };
      served.push(sent);
      return answer(sent);
    }),
  );
  return served;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// Draws the board, and redraws it under other dials with the same cache.
function drawBoard(query: ListQuery) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const board = (dials: ListQuery) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LeadBoard
          query={dials}
          rows={[lead]}
          onMoved={() => undefined}
          hasMore={false}
          loadMore={() => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(board(query));
  return { client, redraw: (dials: ListQuery) => rerender(board(dials)) };
}

// The figure a column states, to the eye or to a screen reader.
const STATED_COUNT =
  ".board-col-count:not([hidden]), .board-col-sub > .sr-only:not([hidden])";

function column(stage: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(
    `.board-col[data-stage="${stage}"]`,
  );
  if (found === null) throw new Error(`no board column for ${stage}`);
  return found;
}

function headOf(stage: string): HTMLElement {
  const head = column(stage).querySelector<HTMLElement>(".board-col-head");
  if (head === null) throw new Error(`no head for ${stage}`);
  return head;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the lead board's terminal columns under the queue's dials", () => {
  it("count only the reader's own leads under Mine", async () => {
    const served = serve(() =>
      json({ rows: [{ status: "promoted", leads: 3 }] }),
    );
    drawBoard(mine());

    await waitFor(() =>
      expect(column("promoted").textContent).toContain("3 leads"),
    );
    // Both stated figures match, so the absent checks below cannot pass vacuously.
    expect(column("promoted").querySelectorAll(STATED_COUNT)).toHaveLength(2);
    const report = served.find((sent) =>
      sent.url.pathname.endsWith("/reports/leads-by-status"),
    );
    expect(report?.body).toMatchObject({ filters: { owner_id: "u-me" } });
  });

  it("withhold the count under a dial the report cannot apply, and name it", async () => {
    const served = serve(() =>
      json({
        data: [{ ...lead, id: "l-q", status: "promoted" }],
        page: { has_more: false, next_cursor: null },
      }),
    );
    drawBoard(mine({ min_score: "80" }));

    const qualified = column("promoted");
    expect(qualified.textContent).toContain(
      en["lead.boardCountsNoScoreFilter"],
    );
    expect(qualified.querySelector(STATED_COUNT)).toBeNull();

    // Opened, it still draws its cards, read under the same dials.
    const head = qualified.querySelector(".board-col-head");
    if (head === null) throw new Error("no Qualified head");
    fireEvent.click(head);
    await waitFor(() =>
      expect(qualified.querySelector('.deal-card[data-lead="l-q"]')).not.toBe(
        null,
      ),
    );
    const read = served.find((sent) => sent.url.pathname.endsWith("/leads"));
    expect(Object.fromEntries(read?.url.searchParams ?? [])).toMatchObject({
      owner_id: "u-me",
      min_score: "80",
      status: "promoted",
      include_archived: "true",
    });
    expect(served.some((sent) => sent.url.pathname.includes("/reports/"))).toBe(
      false,
    );
  });

  it("read a 403 from the report as refused, not as a failure", async () => {
    serve(() =>
      json(
        {
          type: "about:blank",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
        },
        403,
      ),
    );
    drawBoard(mine());

    await waitFor(() =>
      expect(column("disqualified").textContent).toContain(
        en["lead.boardCountsOwnerNotMeasurable"],
      ),
    );
    expect(column("disqualified").querySelector(STATED_COUNT)).toBeNull();
    expect(document.body.textContent).not.toContain(
      en["lead.boardCountsUnavailable"],
    );
  });

  it("state no count while the report has not answered, or failed", async () => {
    serve(() =>
      json(
        { title: "Unavailable", status: 503, code: "service_unavailable" },
        503,
      ),
    );
    drawBoard(mine());

    expect(column("promoted").querySelector(STATED_COUNT)).toBeNull();
    expect(
      await screen.findByText(en["lead.boardCountsUnavailable"]),
    ).toBeTruthy();
    expect(column("promoted").querySelector(STATED_COUNT)).toBeNull();
    expect(column("disqualified").querySelector(STATED_COUNT)).toBeNull();
  });

  it("read a 403 without an owner dial as a failure, not an owner refusal", async () => {
    serve(() =>
      json(
        {
          type: "about:blank",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
        },
        403,
      ),
    );
    drawBoard({ ...UNNARROWED_LIST_QUERY, filters: { status: "promoted" } });

    expect(
      await screen.findByText(en["lead.boardCountsUnavailable"]),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain(
      en["lead.boardCountsOwnerNotMeasurable"],
    );
  });

  it("keep the report's count above leads that did not load", async () => {
    serve((sent) =>
      sent.url.pathname.endsWith("/leads")
        ? json(
            { title: "Unavailable", status: 503, code: "service_unavailable" },
            503,
          )
        : json({ rows: [{ status: "promoted", leads: 5 }] }),
    );
    drawBoard(UNNARROWED_LIST_QUERY);
    await waitFor(() =>
      expect(column("promoted").textContent).toContain("5 leads"),
    );
    fireEvent.click(headOf("promoted"));

    expect(
      await screen.findByText(en["lead.boardTerminalRowsUnavailable"]),
    ).toBeTruthy();
    expect(column("promoted").textContent).toContain("5 leads");
  });

  it("claim no count above a withheld column whose leads did not load, and retry them", async () => {
    let leadReads = 0;
    const served = serve((sent) => {
      if (!sent.url.pathname.endsWith("/leads")) return json({ rows: [] });
      leadReads++;
      return leadReads === 1
        ? json(
            { title: "Unavailable", status: 503, code: "service_unavailable" },
            503,
          )
        : json({
            data: [{ ...lead, id: "l-q", status: "promoted" }],
            page: { has_more: false, next_cursor: null },
          });
    });
    drawBoard(mine({ min_score: "80" }));
    fireEvent.click(headOf("promoted"));

    expect(
      await screen.findByText(en["lead.boardTerminalRowsUnavailableNoCount"]),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain(
      en["lead.boardTerminalRowsUnavailable"],
    );

    fireEvent.click(screen.getByRole("button", { name: en["common.retry"] }));
    await waitFor(() =>
      expect(column("promoted").querySelector('[data-lead="l-q"]')).not.toBe(
        null,
      ),
    );
    expect(leadReads).toBe(2);
    const retried = served.filter((sent) =>
      sent.url.pathname.endsWith("/leads"),
    )[1];
    expect(Object.fromEntries(retried?.url.searchParams ?? [])).toMatchObject({
      owner_id: "u-me",
      min_score: "80",
      status: "promoted",
      include_archived: "true",
    });
    expect(headOf("promoted").getAttribute("aria-expanded")).toBe("true");
    expect(
      screen.queryByText(en["lead.boardTerminalRowsUnavailableNoCount"]),
    ).toBeNull();
  });

  it("fold a column, and its row failure goes with it", async () => {
    serve(() =>
      json(
        { title: "Unavailable", status: 503, code: "service_unavailable" },
        503,
      ),
    );
    drawBoard(mine({ min_score: "80" }));
    fireEvent.click(headOf("promoted"));
    await screen.findByText(en["lead.boardTerminalRowsUnavailableNoCount"]);

    fireEvent.click(headOf("promoted"));
    expect(
      screen.queryByText(en["lead.boardTerminalRowsUnavailableNoCount"]),
    ).toBeNull();
  });

  it("retry the counts after a failure", async () => {
    let reports = 0;
    serve((sent) => {
      if (!sent.url.pathname.includes("/reports/")) return json({ data: [] });
      reports++;
      return reports === 1
        ? json(
            { title: "Unavailable", status: 503, code: "service_unavailable" },
            503,
          )
        : json({ rows: [{ status: "promoted", leads: 4 }] });
    });
    drawBoard(mine());
    await screen.findByText(en["lead.boardCountsUnavailable"]);

    fireEvent.click(screen.getByRole("button", { name: en["common.retry"] }));
    await waitFor(() =>
      expect(column("promoted").textContent).toContain("4 leads"),
    );
    expect(screen.queryByText(en["lead.boardCountsUnavailable"])).toBeNull();
  });

  it("state no count after a refetch of the counts fails", async () => {
    let failing = false;
    serve(() =>
      failing
        ? json(
            { title: "Unavailable", status: 503, code: "service_unavailable" },
            503,
          )
        : json({ rows: [{ status: "promoted", leads: 3 }] }),
    );
    const board = drawBoard(mine());
    await waitFor(() =>
      expect(column("promoted").textContent).toContain("3 leads"),
    );

    failing = true;
    await board.client.invalidateQueries({ queryKey: LEAD_LIST_KEY });
    await screen.findByText(en["lead.boardCountsUnavailable"]);
    expect(column("promoted").querySelector(STATED_COUNT)).toBeNull();
    expect(column("promoted").textContent).not.toContain("3");
  });

  it("keep a cached total out of a column a search withholds", async () => {
    serve(() => json({ rows: [{ status: "promoted", leads: 3 }] }));
    const board = drawBoard(mine());
    await waitFor(() =>
      expect(column("promoted").textContent).toContain("3 leads"),
    );

    board.redraw({ ...mine(), q: "nordwind" });
    expect(column("promoted").textContent).toContain(
      en["lead.boardCountsNoSearch"],
    );
    expect(column("promoted").textContent).not.toContain("3");
  });

  it("claim no count above leads that did not load while the counts failed too", async () => {
    serve(() =>
      json(
        { title: "Unavailable", status: 503, code: "service_unavailable" },
        503,
      ),
    );
    drawBoard(mine());
    await screen.findByText(en["lead.boardCountsUnavailable"]);
    fireEvent.click(headOf("promoted"));

    expect(
      await screen.findByText(en["lead.boardTerminalRowsUnavailableNoCount"]),
    ).toBeTruthy();
    expect(document.body.textContent).not.toContain(
      en["lead.boardTerminalRowsUnavailable"],
    );
  });

  it("drop a counts failure once a dial withholds the counts", async () => {
    serve(() =>
      json(
        { title: "Unavailable", status: 503, code: "service_unavailable" },
        503,
      ),
    );
    const board = drawBoard(mine());
    await screen.findByText(en["lead.boardCountsUnavailable"]);

    board.redraw({ ...mine(), q: "nordwind" });
    expect(column("promoted").textContent).toContain(
      en["lead.boardCountsNoSearch"],
    );
    expect(screen.queryByText(en["lead.boardCountsUnavailable"])).toBeNull();
  });

  it("show no cached cards in a column a status dial excludes", async () => {
    serve(() =>
      json({
        rows: [{ status: "promoted", leads: 1 }],
        data: [{ ...lead, id: "l-q", status: "promoted" }],
        page: { has_more: false, next_cursor: null },
      }),
    );
    const board = drawBoard(mine());
    fireEvent.click(headOf("promoted"));
    await waitFor(() =>
      expect(column("promoted").querySelector('[data-lead="l-q"]')).not.toBe(
        null,
      ),
    );

    board.redraw(mine({ status: "new" }));
    expect(column("promoted").querySelector('[data-lead="l-q"]')).toBeNull();
  });
});
