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
import { MagicPanel } from "../features/sign-in/magic";
import { type Locale, LocaleProvider } from "../i18n";
import type { MagicReceipt } from "../lib/magic.queries";

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
  const [path = ""] = url.split("?");
  return path.endsWith("/magic");
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

export function renderMagic(
  locale: Locale = "en",
  client: QueryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  }),
  visible: Parameters<typeof MagicPanel>[0] = {},
): RenderResult {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <MagicPanel {...visible} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

export { line, receipt } from "./magic.fixtures";
