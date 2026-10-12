/**
 * A Worklist the reader may not open says so, and offers no retry.
 *
 * A named owner with no reach over them answers 403. The screen read that as a
 * transport failure, so the panel said "did not load. Retry" and every press
 * earned the same refusal.
 *
 * @vitest-environment happy-dom
 */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { WorklistScreen } from "./worklist";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("a refused Worklist", () => {
  it("says it is not theirs to read, with no retry", async () => {
    stubTheDay(refusal("permission_denied"));

    renderUnderAToastRegion(ANOTHERS_QUEUE);

    expect(await screen.findByText(en["worklist.owner.refused"])).toBeTruthy();
    // The way back, which is the only thing that can work here.
    expect(
      screen.getByRole("button", { name: en["worklist.owner.backToMine"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: en["state.retry"] }),
    ).toBeNull();
  });

  // The admitting half. A failure that is not a refusal still offers retry,
  // because a reader whose request fell over can usefully press it.
  //
  // A 500 rather than a gateway status: the client retries those, so the
  // surface never settles and the assertion would time out instead of
  // judging anything.
  it("still offers retry when the read simply failed", async () => {
    stubTheDay(
      new Response(
        JSON.stringify({ title: "Internal", status: 500, code: "internal" }),
        {
          status: 500,
          headers: { "content-type": "application/problem+json" },
        },
      ),
    );

    renderUnderAToastRegion(ANOTHERS_QUEUE);

    expect(
      await screen.findByRole("button", { name: en["state.retry"] }),
    ).toBeTruthy();
    expect(screen.queryByText(en["worklist.owner.refused"])).toBeNull();
  });
});

function refusal(code: string) {
  return new Response(
    JSON.stringify({ title: "Forbidden", status: 403, code }),
    { status: 403, headers: { "content-type": "application/problem+json" } },
  );
}

// Every call answers the same way. The screen reads more than one endpoint
// before it can draw, and a stub answering only the first would let the rest
// reach the network.
function stubTheDay(answer: Response) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => answer.clone()),
  );
}

// The queue this reader has no reach over, which is what the link in the
// issue names.
const ANOTHERS_QUEUE = "01a0ced5-0000-7000-8000-000000000000";

function renderUnderAToastRegion(opensOn: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <WorklistScreen opensOn={opensOn} />
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}
