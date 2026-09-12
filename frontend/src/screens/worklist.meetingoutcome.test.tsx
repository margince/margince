// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A meeting that already happened, answered from the row that asked.
//
// The row used to state an obligation and offer nothing: it said a meeting owed
// an answer, and the answer lives on the activity, which the queue did not
// reach into. The backend had accepted `meeting_status` on PATCH /activities
// since the field existed — so the one moment a human knows how it went was the
// one moment nothing on screen could be told.

/** @vitest-environment happy-dom */
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

function aMeetingOwedAnAnswer(over = {}) {
  return row({
    id: "01a05500-0000-7000-8000-0000000000b1",
    source: "meeting_outcome",
    category: "meetings",
    title: "Discovery call with Turbinenbau",
    band: "keep_momentum",
    destination: "today",
    actions: ["decide"],
    ...over,
  });
}

function aDayWithOne(over = {}) {
  return day({
    queue: [aMeetingOwedAnAnswer(over)],
    summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
  });
}

describe("a meeting that owes an answer", () => {
  it("offers all three outcomes, not one behind a menu", async () => {
    stub(aDayWithOne());
    renderUnderAToastRegion();

    await screen.findByText(/Discovery call with Turbinenbau/);
    // Three equally likely answers. Hiding two behind a chevron would make the
    // common case a second click on a row whose whole purpose is one answer.
    expect(
      await screen.findByRole("button", { name: /it happened/i }),
    ).not.toBeNull();
    expect(screen.getByRole("button", { name: /didn't come/i })).not.toBeNull();
    expect(screen.getByRole("button", { name: /called off/i })).not.toBeNull();
  });

  it("records the outcome the reader pressed, not a fixed one", async () => {
    // The assertion is on the PATCH body. Three buttons that all send "held"
    // would satisfy every visible check on the screen while recording the
    // wrong thing about two meetings in three.
    const sent: unknown[] = [];
    stub(aDayWithOne());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (/\/activities\/[^/]+$/.test(url.split("?")[0])) {
          // openapi-fetch hands the stub a Request object, not (url, init), so
          // the body is read off the request rather than off init — which is
          // always undefined here and would capture nothing.
          const body =
            input instanceof Request
              ? await input.clone().text()
              : String(init?.body ?? "");
          if (body !== "") {
            sent.push(JSON.parse(body));
          }
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: /didn't come/i }),
    );
    expect(
      await screen.findByText(/Recorded how the meeting went/i),
    ).not.toBeNull();
    expect(sent).toEqual([{ meeting_status: "no_show" }]);
  });

  it("says so when the write is refused, rather than falling silent", async () => {
    stub(aDayWithOne());
    const passthrough = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input instanceof Request ? input.url : input);
        if (/\/activities\/[^/]+$/.test(url.split("?")[0])) {
          return new Response(JSON.stringify({ title: "no" }), {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          });
        }
        return passthrough(input, init);
      }),
    );
    const user = userEvent.setup();
    renderUnderAToastRegion();

    await user.click(
      await screen.findByRole("button", { name: /it happened/i }),
    );
    // A refused write leaves the row exactly as it was, which renders the same
    // as a click that did nothing.
    expect(await screen.findByText(/could not be recorded/i)).not.toBeNull();
  });
});

// The confirmation and the refusal both live in a toast, and renderWorklist
// mounts no region — the testkit is production-shaped for the conformance gate,
// which allows one ToastProvider and one ToastRegion and names main.tsx as
// their home. Mounted here, in a file the gate does not scan.
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
