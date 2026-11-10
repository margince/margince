/** @vitest-environment happy-dom */
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { AppErrorBoundary } from "./errorboundary";
import { createQueryClient } from "./queryclient";

// Without a boundary the first throw in any screen takes the entire document
// with it, and the reader gets a white page. These cases are the difference
// between that and a view they can retry.

const THROWN = "cannot read properties of undefined";
const SHELL = "the routed shell";
const TITLE = "This view stopped working";
const RETRY = "Retry";

let broken = true;

function Fragile() {
  if (broken) {
    throw new Error(THROWN);
  }
  return <p>{SHELL}</p>;
}

function renderBoundary(child: ReactNode, client = createQueryClient()) {
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AppErrorBoundary>{child}</AppErrorBoundary>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  broken = true;
  // React logs every error a boundary catches, with its component stack. The
  // throws below are the point of the suite, so that output is noise — and a
  // suite whose output is noise is one nobody reads when it matters.
  vi.spyOn(console, "error").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("the app-level error boundary", () => {
  it("replaces a thrown render with a surface that says what to do", () => {
    renderBoundary(<Fragile />);

    expect(screen.getByRole("alert").textContent).toContain(TITLE);
    expect(screen.getByRole("button", { name: RETRY })).toBeTruthy();
    expect(screen.queryByText(SHELL)).toBeNull();
  });

  it("shows the reader nothing of the error itself", () => {
    renderBoundary(<Fragile />);

    expect(document.body.textContent).not.toContain(THROWN);
    expect(screen.queryByText(/Error|undefined|at Fragile/)).toBeNull();
  });

  it("renders the screen again when the reader retries", async () => {
    const user = userEvent.setup();
    renderBoundary(<Fragile />);

    broken = false;
    await user.click(screen.getByRole("button", { name: RETRY }));

    expect(await screen.findByText(SHELL)).toBeTruthy();
    expect(screen.queryByText(TITLE)).toBeNull();
  });

  // The pairing with the query cache's reset boundary is what makes the retry
  // real: a query keeps its failed result, so without the reset the button
  // would clear the boundary's state and be handed the same error back.
  it("clears a failed query on retry, so the data is fetched again", async () => {
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    function FragileQuery() {
      const { data } = useQuery({
        queryKey: ["boundary-pairing"],
        queryFn: () => {
          if (broken) {
            throw new Error(THROWN);
          }
          return Promise.resolve(SHELL);
        },
        throwOnError: true,
      });
      return <p>{data}</p>;
    }

    renderBoundary(<FragileQuery />, client);

    expect(await screen.findByRole("button", { name: RETRY })).toBeTruthy();

    broken = false;
    await user.click(screen.getByRole("button", { name: RETRY }));

    expect(await screen.findByText(SHELL)).toBeTruthy();
  });

  // A success the view cannot draw is held fresh by the cache, so a remount
  // alone would serve it again and throw again.
  it("reads again an answer the cache holds as a success", async () => {
    const user = userEvent.setup();
    const answers = [{}, {}, { rows: [SHELL] }];
    const queryFn = vi.fn(() => Promise.resolve(answers.shift()));

    function Rows() {
      const { data } = useQuery({ queryKey: ["boundary-rows"], queryFn });
      if (!data) return null;
      // The property access a malformed answer fails, as a screen's would.
      const rows = (data as { rows: string[] }).rows;
      return <p>{rows.join(", ")}</p>;
    }

    renderBoundary(<Rows />);
    await user.click(await screen.findByRole("button", { name: RETRY }));
    await user.click(await screen.findByRole("button", { name: RETRY }));

    expect(await screen.findByText(SHELL)).toBeTruthy();
    // One read per Retry, and none on its own between them.
    expect(queryFn).toHaveBeenCalledTimes(3);
  });
  // A deploy replaced the screen's code under a tab opened before it. Retrying
  // inside the page re-runs the failed import, so the boundary reloads instead.
  it("reloads onto the new build when a screen's code is gone", () => {
    sessionStorage.clear();
    const reload = vi.fn();
    vi.spyOn(window, "location", "get").mockReturnValue({
      ...window.location,
      reload,
    });
    function Stale(): never {
      throw new TypeError(
        "Failed to fetch dynamically imported module: /assets/settings-a1.js",
      );
    }
    renderBoundary(<Stale />);

    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("lets an explicit Retry reload after the automatic chunk-reload guard fired", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-22T12:00:00Z"));
    sessionStorage.clear();
    const reload = vi.fn();
    vi.spyOn(window, "location", "get").mockReturnValue({
      ...window.location,
      reload,
    });
    function Stale(): never {
      throw new TypeError(
        "Failed to fetch dynamically imported module: /assets/settings.js",
      );
    }
    renderBoundary(<Stale />);
    expect(reload).toHaveBeenCalledTimes(1);
    await userEvent.setup().click(screen.getByRole("button", { name: RETRY }));
    expect(reload).toHaveBeenCalledTimes(2);
  });

  it("does not reload for a defect in the screen", () => {
    const reload = vi.fn();
    vi.spyOn(window, "location", "get").mockReturnValue({
      ...window.location,
      reload,
    });
    renderBoundary(<Fragile />);

    expect(reload).not.toHaveBeenCalled();
  });
});
