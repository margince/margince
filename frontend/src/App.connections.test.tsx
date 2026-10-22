/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { createQueryClient } from "./app/queryclient";
import { LocaleProvider } from "./i18n";
import { connectivityNow } from "./lib/connectivity";
import { memoryStorage } from "./testing/appharness";
import { meFixture } from "./testing/mefixture";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Unconditionally, not at the end of the case that installed a spy. A case
  // that fails its assertion never reaches its own teardown, and a leaked
  // console spy silences the rest of the file. That looks like a suite that
  // passes.
  vi.restoreAllMocks();
});

// A reader who opens the app while the api restarts gets the connection screen,
// and must get in without pressing anything once Margince answers again.
describe("the connection screen after Margince could not be reached", () => {
  const CONNECTION_TITLE = "Margince could not be reached";
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  let apiUp = false;
  let sessionAnswer = () => json(meFixture({}));
  let sessionReads = 0;

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    apiUp = false;
    sessionAnswer = () => json(meFixture({}));
    sessionReads = 0;
    vi.stubGlobal("localStorage", memoryStorage());
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = input instanceof Request ? input.url : String(input);
        if (url.endsWith("/v1/me")) sessionReads += 1;
        if (!apiUp) throw new TypeError("Failed to fetch");
        if (url === "/healthz") return new Response("ok");
        if (url.endsWith("/v1/me")) return sessionAnswer();
        return json({ code: "service_unavailable" }, 503);
      }),
    );
  });

  afterEach(() => {
    // Unmounted first: a shell still reading would reach the real network.
    cleanup();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  async function openWhileUnreachable() {
    rtlRender(
      <QueryClientProvider client={createQueryClient()}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText(CONNECTION_TITLE)).toBeTruthy();
    // One check, sent while the screen watches, is what gives it an outage to
    // hold; the reads it then pauses cannot send another.
    await waitFor(() => expect(connectivityNow()).toBe("unreachable"));
    expect(sessionReads).toBe(2);
  }

  it("lets the reader in by itself once Margince answers", async () => {
    await openWhileUnreachable();

    apiUp = true;
    await act(() => vi.advanceTimersByTimeAsync(2_000));

    await waitFor(() =>
      expect(screen.queryByText(CONNECTION_TITLE)).toBeNull(),
    );
    expect(sessionReads).toBe(3);
  });

  it("lets the reader in at once when they press Retry", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    await openWhileUnreachable();

    apiUp = true;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() =>
      expect(screen.queryByText(CONNECTION_TITLE)).toBeNull(),
    );
  });

  // Answered, but with no session in it: the boundary draws this same screen,
  // so the check must not send the reader round again.
  it("checks once and stays when Margince answers with no session", async () => {
    apiUp = true;
    sessionAnswer = () => json({});
    rtlRender(
      <QueryClientProvider client={createQueryClient()}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText(CONNECTION_TITLE)).toBeTruthy();

    await act(() => vi.advanceTimersByTimeAsync(20_000));
    expect(sessionReads).toBe(2);
    expect(screen.getByText(CONNECTION_TITLE)).toBeTruthy();
  });

  it("gives way to sign-in when its own check is answered 401", async () => {
    apiUp = true;
    let answers = 0;
    sessionAnswer = () => {
      answers += 1;
      if (answers === 1) throw new TypeError("Failed to fetch");
      return json({ status: 401, code: "unauthorized" }, 401);
    };
    rtlRender(
      <QueryClientProvider client={createQueryClient()}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(await screen.findByText(CONNECTION_TITLE)).toBeTruthy();

    expect(
      await screen.findByRole("heading", { name: "Sign in to Margince" }),
    ).toBeTruthy();
  });
});
