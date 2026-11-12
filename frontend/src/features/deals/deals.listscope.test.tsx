/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { listFetchLimit } from "../../components/listquery";
import { DealsScreen } from "./deals";
import { deal, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// What the screen asked for, read off the wire: the deals reads by URL and the
// stage totals by the report body they posted.
function recordReads(options: { failPipelines?: boolean } = {}) {
  const base = stubBackend([deal({})]);
  const dealReads: URL[] = [];
  const totalsBodies: { filters?: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const url = new URL(request.url, "https://test.local");
      if (options.failPipelines && url.pathname.endsWith("/pipelines")) {
        return new Response(JSON.stringify({ status: 500, title: "down" }), {
          status: 500,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      if (url.pathname.endsWith("/deals")) {
        dealReads.push(url);
      }
      if (url.pathname.endsWith("/reports/deals-by-stage")) {
        totalsBodies.push(await request.clone().json());
      }
      return base(request);
    }),
  );
  return { dealReads, totalsBodies };
}

// The pipeline a list is scoped to arrives with the pipelines read. Asking
// before it lands asks for every deal, an answer thrown away when the scope
// appears.
describe("the deals list asks only once it knows its pipeline", () => {
  it("reads the table once, for that pipeline", async () => {
    window.location.hash = "#/deals?view=table";
    const { dealReads } = recordReads();
    render(<DealsScreen />);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(dealReads.map((url) => url.searchParams.get("pipeline_id"))).toEqual(
      ["pl"],
    );
  });

  it("asks for the board's stage totals only within that pipeline", async () => {
    const { totalsBodies } = recordReads();
    render(<DealsScreen />);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    await waitFor(() => expect(totalsBodies.length).toBeGreaterThan(0));
    expect(totalsBodies.map((body) => body.filters?.pipeline_id)).toEqual(
      totalsBodies.map(() => "pl"),
    );
  });

  it("reads the table two rendered pages at a time, at the reader's page size", async () => {
    window.location.hash = "#/deals?view=table&per=50";
    const { dealReads } = recordReads();
    render(<DealsScreen />);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(dealReads.map((url) => url.searchParams.get("limit"))).toEqual([
      String(listFetchLimit(50)),
    ]);
  });

  it("asks for no stage totals while the table shows, since it draws none", async () => {
    window.location.hash = "#/deals?view=table";
    const { totalsBodies } = recordReads();
    render(<DealsScreen />);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(totalsBodies).toEqual([]);
  });

  // The table waits on the pipelines read, so that read failing is the
  // table's failure. Otherwise it would show a list loading forever.
  it("shows the table's failure when the pipelines read fails", async () => {
    window.location.hash = "#/deals?view=table";
    const { dealReads } = recordReads({ failPipelines: true });
    render(<DealsScreen />);
    expect(
      await screen.findByRole("button", { name: "Retry" }),
    ).toBeInTheDocument();
    expect(dealReads).toEqual([]);
  });
});
