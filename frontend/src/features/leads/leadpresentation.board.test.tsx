/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import type { ListQuery } from "../../components/listquery";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
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
    q: "",
    sort: "",
    includeArchived: false,
    filters: { owner_id: "u-me", ...filters },
    perPage: 25,
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

function drawBoard(query: ListQuery) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <LeadBoard
          query={query}
          rows={[lead]}
          onMoved={() => undefined}
          hasMore={false}
          loadMore={() => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The figure a column states, to the eye or to a screen reader.
const STATED_COUNT =
  ".board-col-count:not([hidden]), .board-col-total > .sr-only:not([hidden])";

function column(stage: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(
    `.board-col[data-stage="${stage}"]`,
  );
  if (found === null) throw new Error(`no board column for ${stage}`);
  return found;
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
});
