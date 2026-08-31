/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type Locale, LocaleProvider } from "../i18n";
import { WorklistScreen } from "./worklist";

// The ranked queue, and the ways it can mislead the person reading it.
//
// Every case here is one promise the page makes: that the order is readable,
// that a figure describes the rows beneath it, that nothing is drawn to report
// a zero, and that the database's own words never reach the screen.

type Worklist = components["schemas"]["Worklist"];
type WorklistItem = components["schemas"]["WorklistItem"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function stub(day: Worklist) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/worklist")) {
        return jsonResponse(day);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function renderWorklist(locale: Locale = "en") {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <WorklistScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function day(over: Partial<Worklist> = {}): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    queue: [],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    ...over,
  };
}

function row(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "row-1",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    because: [],
    actions: [],
    ...over,
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("what the ranked queue tells a reader", () => {
  it("draws no panel to report a zero", async () => {
    stub(day());
    const { container } = renderWorklist();

    await screen.findByText("Nothing is waiting on you.");

    // Topology, not headings: a panel drawn to say "none" is the thing the
    // concept asks us to remove, whatever words it carries.
    expect(container.querySelectorAll(".panel")).toHaveLength(0);
  });

  it("says what happens if the reader does nothing", async () => {
    stub(
      day({
        queue: [row({ title: "Send the proposal" })],
        summary: { urgent: 0, due: 1, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText("If you do nothing, it slips."),
    ).toBeTruthy();
  });

  it("says why a row sits above the one below it", async () => {
    stub(
      day({
        queue: [
          row({
            id: "a",
            title: "Closing tomorrow",
            above_next: {
              comparator: "deadline",
              mine: { kind: "date", date: "2026-09-01T09:00:00Z" },
              theirs: { kind: "date", date: "2027-05-01T09:00:00Z" },
            },
          }),
          row({ id: "b", title: "Closing later" }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 2 },
      }),
    );
    renderWorklist();

    expect(await screen.findByText(/Above the next:/)).toBeTruthy();
  });

  it("never prints the database's own words at a reader", async () => {
    stub(
      day({
        queue: [
          row({
            id: "raw",
            source: "approval",
            category: "decisions",
            level: 6,
            consequence: "data_drifts",
            kind: "capture_counterparty_verdict",
            because: [{ kind: "routine" }],
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("A decision is waiting");
    // The kind travels for the icon and the label; it must never be the label.
    expect(container.textContent).not.toContain("capture_counterparty_verdict");
    expect(container.textContent).not.toContain("_");
  });

  it("writes the day's figures in the reader's own notation", async () => {
    stub(
      day({
        queue: [row()],
        summary: { urgent: 1234, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist("de");

    expect(await screen.findByText(/1\.234/)).toBeTruthy();
  });

  it("offers no scope control when the reader has one scope", async () => {
    stub(day({ queue: [row()] }));
    renderWorklist();

    await screen.findByText("A task");
    expect(screen.queryByRole("radiogroup", { name: "Whose work" })).toBeNull();
  });

  it("offers the scope control when the reader may ask for more", async () => {
    stub(day({ queue: [row()], scope_options: ["mine", "team", "all"] }));
    renderWorklist();

    await screen.findByText("A task");
    expect(screen.getAllByText("My team").length).toBeGreaterThan(0);
  });

  it("warns rather than claiming a clear day it could not read", async () => {
    stub(
      day({
        sources_unavailable: [{ source: "capture_health", reason: "failed" }],
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText(
        "Nothing is waiting among the sources that answered.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Nothing is waiting on you.")).toBeNull();
  });

  it("narrows the queue when the reader picks a kind of work", async () => {
    const user = userEvent.setup();
    stub(day({ queue: [row({ title: "A waiting customer" })] }));
    renderWorklist();

    await screen.findByText("A waiting customer");
    await user.click(screen.getByRole("button", { name: /Deals at risk/ }));

    await waitFor(() => {
      const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls;
      // The client passes a Request, so the URL is read off it rather than
      // stringified — String(request) yields "[object Request]".
      const urls = calls.map((call) => {
        const target = call[0];
        return target instanceof Request ? target.url : String(target);
      });
      expect(urls.some((url) => url.includes("filter=deals_at_risk"))).toBe(
        true,
      );
    });
  });
});
