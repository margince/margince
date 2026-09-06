// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Saying a promise was kept, from the row that keeps asking for it.
//
// The claim's status column has carried open/done/dismissed since the table
// existed and every reader filters on `open`. Nothing wrote the other two, so
// the row named a debt every morning and there was no way to say it was paid.

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";
import { day, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function aPromise(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000f1",
    source: "conversation_claim",
    category: "customer_waiting",
    title: "Send the updated quote by Friday",
    band: "now",
    destination: "today",
    actions: ["complete", "open"],
    subject: { type: "person", id: "01a05500-0000-7000-8000-0000000000f2" },
    ...over,
  });
}

function aDayWithOne(over = {}) {
  return day({
    queue: [aPromise(over)],
    summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
  });
}

describe("a promise the reader made", () => {
  it("can be marked kept from the row", async () => {
    stub(aDayWithOne());
    renderUnderAToastRegion();

    await screen.findByText(/Send the updated quote/);
    expect(
      await screen.findByRole("button", { name: /^done$/i }),
    ).not.toBeNull();
  });

  it("settles it as done, not as never-promised", async () => {
    // The endpoint takes both outcomes and they mean different things: `done`
    // says the thing happened, `dismissed` says it was never a promise. A queue
    // row that sent the wrong one would record the rep as having misread their
    // own conversation.
    const sent: unknown[] = [];
    stub(aDayWithOne());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/claims/") && url.endsWith("/settle")) {
          const body =
            input instanceof Request
              ? await input.clone().text()
              : String(init?.body ?? "");
          sent.push(JSON.parse(body));
          return new Response(null, { status: 204 });
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /^done$/i }));
    expect(await screen.findByText(/marked as kept/i)).not.toBeNull();
    expect(sent).toEqual([{ outcome: "done" }]);
  });

  it("says so when the settle is refused, rather than falling silent", async () => {
    stub(aDayWithOne());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/claims/") && url.endsWith("/settle")) {
          return new Response(JSON.stringify({ title: "no" }), {
            status: 409,
            headers: { "content-type": "application/problem+json" },
          });
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(await screen.findByRole("button", { name: /^done$/i }));
    expect(
      await screen.findByText(/could not be marked as kept/i),
    ).not.toBeNull();
  });
});

// Both answers live in a toast, and renderWorklist mounts no region: the
// testkit is production-shaped for the conformance gate, which allows one
// ToastProvider and one ToastRegion and names main.tsx as their home.
function renderUnderAToastRegion() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
