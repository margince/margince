// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The harness `magic.test.tsx` renders the receipt through.
 *
 * A `.tsx` module rather than a `.test.tsx` one, so vitest does not collect it
 * as a suite of its own with no cases in it, and so the design-system and lint
 * gates hold it to the app's rules the way they hold `brief.testkit.tsx`.
 */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type RenderResult, render } from "@testing-library/react";
import { vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { MagicPanel } from "./magic";
import type { MagicLine, MagicReceipt } from "./magic.queries";

export function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// The receipt's own read, told apart by the EXACT path. A prefix match would
// also answer anything nested under it, and a panel reading a body meant for a
// sibling route throws inside React's render, where no assertion sees it.
function isMagicRead(url: string): boolean {
  return url.split("?")[0].endsWith("/magic");
}

/** The receipt, and an empty page for every other read the panel does not make. */
export function stub(answer: MagicReceipt) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      return isMagicRead(url)
        ? jsonResponse(answer)
        : jsonResponse({ data: [] });
    }),
  );
}

/** A read that refuses, so the panel's failed arm is reached the way a server reaches it. */
export function stubRefusal(status = 500) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ title: "Server Error", status }, status)),
  );
}

/** A read that never settles, so the loading arm stays up for the assertion. */
export function stubPending() {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => new Promise<Response>(() => {})),
  );
}

export function renderMagic(locale: Locale = "en"): RenderResult {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <MagicPanel />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/**
 * The quietest receipt the server can send: every lane present and empty.
 *
 * Every field the contract marks required is here, so a case overriding one
 * lane still describes a payload production could produce — a fixture missing
 * `totals` or `sources_unavailable` would test a shape no server sends.
 */
export function receipt(over: Partial<MagicReceipt> = {}): MagicReceipt {
  return {
    as_of: "2026-09-13T08:00:00Z",
    since: "2026-09-12T08:00:00Z",
    done: [],
    needs_you: [],
    could_not_complete: [],
    watching: [],
    totals: { done: 0, needs_you: 0, could_not_complete: 0, watching: 0 },
    not_shown: [],
    sources_unavailable: [],
    ...over,
  };
}

/** One line, in the `done` lane unless a case says otherwise. */
export function line(over: Partial<MagicLine> = {}): MagicLine {
  return {
    id: "00000000-0000-7000-8000-000000000001",
    occurred_at: "2026-09-13T07:30:00Z",
    lane: "done",
    summary: { key: "magic.action.advance_stage" },
    actor: { type: "agent", id: "runner" },
    undo: { undoable: false, reason: "not_a_replayable_verb" },
    ...over,
  };
}
