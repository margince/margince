/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import { companyBackstop, jsonResponse, stubFetch } from "./company.fixtures";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// The record's context column belongs to the SHELL, not the record, and the
// record fills it through a portal — so this carries the real region rather
// than a stand-in. A test that supplied its own column would prove nothing
// about the one the product draws.
function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The deep read (A102/R2): one click starts a background whole-site crawl and
// the card polls the read report until it lands on a terminal status. The
// report is the transparency surface — a partial crawl must SAY it stopped
// early and name every skipped page's reason, and staged proposals point at
// the approvals inbox.
const runningRead = {
  read_id: "rd-1",
  company_id: "o-1",
  seed_url: "https://brandt.example",
  status: "running",
  status_code: null,
  status_detail: null,
  next_attempt_at: null,
  pages: [
    { url: "https://brandt.example/", kind: "home" },
    { url: "https://brandt.example/team", kind: "team" },
  ],
  skipped: [],
  proposal_ids: [],
  created_at: "2026-07-17T08:00:00Z",
};

function stubDeepRead(options: {
  post?: () => Response;
  report?: () => Response;
  /**
   * What `/site-reads/latest` answers before anything is started. The default is
   * 404 — this account has never been read — because that is the state the
   * offer is written for, and it is what decides whether the panel pitches the
   * capability or reports on a read that already ran.
   */
  latest?: () => Response;
}) {
  // Only the requests this suite answers itself are recorded: what the deep read
  // does is a sequence of POSTs and report polls, and the page-shell reads the
  // shared backstop serves are not part of that sequence.
  const calls: string[] = [];
  stubFetch(async (url, method) => {
    const { pathname } = new URL(url);
    calls.push(`${method} ${pathname}`);
    if (method === "POST" && pathname.endsWith("/deep-read")) {
      return (
        options.post ??
        (() => jsonResponse({ read_id: "rd-1", status: "queued" }, 202))
      )();
    }
    if (pathname.endsWith("/site-reads/latest")) {
      return (options.latest ?? (() => new Response(null, { status: 404 })))();
    }
    if (pathname.includes("/site-reads/")) {
      return (options.report ?? (() => jsonResponse(runningRead)))();
    }
    return companyBackstop(url);
  });
  return { calls };
}

async function startDeepRead(calls: string[]) {
  await waitFor(() =>
    expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
  );
  // No navigation: this fixture's account has nothing on file, so the 360
  // leads with the research offer rather than filing it under the record's
  // tools. An account that HAS something meets it on Profile instead, and the
  // offer renders in exactly one of the two.
  await userEvent.click(
    screen.getByRole("button", { name: "Start company research" }),
  );
  await waitFor(() =>
    expect(
      calls.some(
        (call) =>
          call.startsWith("POST") && call.endsWith("/companies/o-1/deep-read"),
      ),
    ).toBe(true),
  );
}

describe("company-360 deep read", () => {
  it("POSTs deep-read on click and polls the read report every 3s while running", async () => {
    const { calls } = stubDeepRead({});
    const reportCalls = () =>
      calls.filter((call) => call.endsWith("/companies/o-1/site-reads/rd-1"))
        .length;
    // The whole flow runs on fake timers so react-query's 3s poll interval is
    // scheduled on the fake clock (a poll timer armed on the real clock could
    // not be advanced). Each advance flushes due timers plus the microtask
    // chains behind the stubbed fetches.
    const flush = () =>
      act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
    vi.useFakeTimers();
    try {
      render(<CompanyScreen id="o-1" />);
      await flush();
      await flush();
      // No navigation: this fixture's account has nothing on file, so the 360
      // itself leads with the research offer rather than filing it under the
      // record's tools.
      fireEvent.click(
        screen.getByRole("button", { name: "Start company research" }),
      );
      await flush();
      await flush();
      expect(
        calls.some(
          (call) =>
            call.startsWith("POST") &&
            call.endsWith("/companies/o-1/deep-read"),
        ),
      ).toBe(true);
      // A running report renders pages-so-far progress…
      expect(reportCalls()).toBe(1);
      expect(screen.getByText("2 pages read so far")).toBeTruthy();
      // …and keeps polling: the 3s interval fires another report fetch.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(3000);
      });
      expect(reportCalls()).toBe(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("shows a budget deferral as an automatic resume, not a failed read", async () => {
    const { calls } = stubDeepRead({
      report: () =>
        jsonResponse({
          ...runningRead,
          status: "deferred",
          status_code: "budget_deferred",
          status_detail:
            "AI budget reached its current limit. This website read will resume automatically.",
          next_attempt_at: "2026-08-01T00:00:00Z",
        }),
    });
    render(<CompanyScreen id="o-1" />);
    await startDeepRead(calls);

    await waitFor(() =>
      expect(screen.getByText("Waiting for AI budget")).toBeTruthy(),
    );
    expect(
      screen.getByText(/This website read will resume automatically/),
    ).toBeTruthy();
    expect(screen.getByText(/Resumes automatically/)).toBeTruthy();
    expect(screen.queryByText("Failed")).toBeNull();
  });

  it("a page cap reads as the size the read was given, not as a fault", async () => {
    const { calls } = stubDeepRead({
      report: () =>
        jsonResponse({
          ...runningRead,
          status: "partial",
          stopped_reason: "page_cap",
          fact_count: 6,
          skipped: [
            { url: "https://brandt.example/careers", reason: "robots" },
            { url: "https://elsewhere.example/profile", reason: "off_domain" },
          ],
          finished_at: "2026-07-17T08:04:00Z",
        }),
    });
    render(<CompanyScreen id="o-1" />);
    await startDeepRead(calls);

    // The page cap is the size this read was CONFIGURED for, so it is stated as
    // what the read did rather than as a warning about what it did not: no
    // "Stopped early" anywhere, and no warn badge repeating the cap underneath.
    await waitFor(() =>
      expect(screen.getByText("Read up to the page limit")).toBeTruthy(),
    );
    expect(screen.queryByText(/Stopped early/)).toBeNull();
    expect(screen.getByText("6 evidenced facts staged")).toBeTruthy();
    // The crawl's own URL lists are debug output, not something a contact
    // reading a company record has any use for.
    expect(screen.queryByText("Pages skipped")).toBeNull();
    expect(screen.queryByText("brandt.example/careers")).toBeNull();
  });

  it("a budget stop keeps its warning, because a later run may get further", async () => {
    // The other half of the page-cap rule. Losing the model budget is not a
    // size this read was given, so it stays a warning: re-running it later is
    // a thing a rep can usefully do, which a page cap never is.
    const { calls } = stubDeepRead({
      report: () =>
        jsonResponse({
          ...runningRead,
          status: "partial",
          stopped_reason: "budget",
          fact_count: 3,
          finished_at: "2026-07-17T08:04:00Z",
        }),
    });
    render(<CompanyScreen id="o-1" />);
    await startDeepRead(calls);

    await waitFor(() =>
      expect(screen.getByText("Stopped early: model budget")).toBeTruthy(),
    );
    expect(screen.queryByText("Read up to the page limit")).toBeNull();
  });

  it("pitches the research offer only until a read exists", async () => {
    // The panel does two jobs and must not do both at once: it explains a
    // capability nobody here has used, then reports on the read that answered
    // the offer. A pitch drawn over a finished read tells a rep the site was
    // never looked at.
    stubDeepRead({
      latest: () =>
        jsonResponse({
          ...runningRead,
          status: "done",
          fact_count: 9,
          finished_at: "2026-07-17T08:05:00Z",
        }),
    });
    render(<CompanyScreen id="o-1" />);

    await waitFor(() =>
      expect(screen.getByText("Website research")).toBeTruthy(),
    );
    expect(screen.queryByText("Margince can fill this in")).toBeNull();
    expect(screen.queryByText(/It reads the company's website/)).toBeNull();
    expect(
      screen.getByRole("button", { name: "Read the website again" }),
    ).toBeTruthy();
  });

  it("a done report links staged leads to the inbox and lists no crawl URLs", async () => {
    const { calls } = stubDeepRead({
      report: () =>
        jsonResponse({
          ...runningRead,
          status: "done",
          fact_count: 9,
          proposal_ids: ["ap-1", "ap-2"],
          finished_at: "2026-07-17T08:05:00Z",
        }),
    });
    render(<CompanyScreen id="o-1" />);
    await startDeepRead(calls);

    await waitFor(() =>
      expect(
        screen.getByText("2 proposals waiting for your review"),
      ).toBeTruthy(),
    );
    // A complete crawl carries no stopped-early banner.
    expect(screen.queryByText(/Stopped early:/)).toBeNull();
    expect(screen.queryByText("Pages read")).toBeNull();
    expect(screen.queryByText("brandt.example/team")).toBeNull();

    await userEvent.click(
      screen.getByRole("button", { name: "Open the Worklist" }),
    );
    expect(window.location.hash).toBe("#/worklist");
  });

  it("renders the honest 422 detail when the company has no website on file", async () => {
    stubDeepRead({
      post: () =>
        jsonResponse(
          { title: "Unprocessable", detail: "no website on file" },
          422,
        ),
    });
    render(<CompanyScreen id="o-1" />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Start company research" }),
    );
    await waitFor(() =>
      expect(screen.getByText("no website on file")).toBeTruthy(),
    );
  });

  it("names the unwired seam on a 501 instead of a generic failure", async () => {
    stubDeepRead({
      post: () => jsonResponse({ title: "Not Implemented" }, 501),
    });
    render(<CompanyScreen id="o-1" />);
    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Start company research" }),
    );
    await waitFor(() =>
      expect(
        screen.getByText("Site reading is not configured on this server."),
      ).toBeTruthy(),
    );
  });
});
