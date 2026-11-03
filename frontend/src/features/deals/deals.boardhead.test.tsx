/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StageColumnRead } from "./dealboard.queries";
import { DealsScreen } from "./deals";
import { StageColumnFoot } from "./deals.board";
import { deal, jsonResponse, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

const twoDeals = [
  deal({ id: "a", name: "Fleet one", stage_id: "s1" }),
  deal({ id: "b", name: "Fleet two", stage_id: "s2" }),
];

const reportRows = [
  { stage_id: "s1", deals: 250 },
  { stage_id: "s2", deals: 10 },
];

// The deals backend, with the requests `waits` names answered only once it
// lets them through.
function backendWith(
  base: ReturnType<typeof stubBackend>,
  waits: (
    url: URL,
    method: string,
  ) => Promise<Response | undefined> | undefined,
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t.l");
    const method = request ? request.method : (init?.method ?? "GET");
    return (await waits(url, method)) ?? base(input, init);
  });
}

// The surface's own count, not a column's spoken one.
const headCount = (text: string) =>
  screen.queryAllByText(text).filter((node) => !node.closest("[data-stage]"));

const column = (stageId: string) => {
  const node = document.querySelector<HTMLElement>(`[data-stage="${stageId}"]`);
  if (!node) throw new Error(`the board drew no ${stageId} column`);
  return node;
};

describe("the deals board's counts", () => {
  it.each([
    { name: "report in play", hash: "#/deals", head: "260 deals" },
    { name: "totals withheld", hash: "#/deals?q=fleet", head: "2 deals" },
  ])("states the board's head count with the $name", async ({ hash, head }) => {
    window.location.hash = hash;
    vi.stubGlobal(
      "fetch",
      stubBackend(twoDeals, { stageTotalsRows: reportRows }),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet two")).toBeTruthy();
    await waitFor(() => expect(headCount(head)).toHaveLength(1));
  });

  it("states no count while the pipelines fail", async () => {
    vi.stubGlobal(
      "fetch",
      backendWith(stubBackend(twoDeals), (url) =>
        url.pathname.endsWith("/pipelines")
          ? Promise.resolve(jsonResponse({ code: "internal" }, 500))
          : undefined,
      ),
    );
    render(<DealsScreen />);

    expect(
      await screen.findByText("Could not load this view. Reload the page."),
    ).toBeTruthy();
    expect(screen.queryByText(/^\d+ deals?$/)).toBeNull();
  });

  // Counted from the cards first and the report after, the head would jump.
  it("waits for the stage report before it states the head count", async () => {
    let answerReport = () => {};
    const report = new Promise<void>((resolve) => {
      answerReport = resolve;
    });
    const base = stubBackend(twoDeals, { stageTotalsRows: reportRows });
    vi.stubGlobal(
      "fetch",
      backendWith(base, (url, method) =>
        method === "POST" && url.pathname.includes("/reports/")
          ? report.then(() => undefined)
          : undefined,
      ),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet two")).toBeTruthy();
    expect(headCount("2 deals")).toEqual([]);
    answerReport();

    await waitFor(() => expect(headCount("260 deals")).toHaveLength(1));
  });

  it("says the head counts only the loaded cards while a column has more", async () => {
    window.location.hash = "#/deals?q=fleet";
    vi.stubGlobal(
      "fetch",
      stubBackend(twoDeals, {
        nextPage: [deal({ id: "c", name: "Fleet three", stage_id: "s1" })],
      }),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("1 to 2 of 2 deals loaded")).toBeTruthy();
  });

  // A zero over a column still reading would say the stage is empty.
  it("states no count over a column whose first read has not answered", async () => {
    vi.stubGlobal(
      "fetch",
      backendWith(stubBackend(twoDeals), (url) =>
        url.searchParams.get("stage_id") === "s3"
          ? new Promise<Response>(() => {})
          : undefined,
      ),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet two")).toBeTruthy();
    expect(
      column("s3").querySelector(".board-col-count:not([hidden])"),
    ).toBeNull();
    expect(
      within(column("s3")).queryByText("0 deals", { ignore: "[hidden]" }),
    ).toBeNull();
    expect(within(column("s1")).getByText("1 deal")).toBeTruthy();
  });

  // The report counts every stage, so a column still reading holds nothing up.
  it("states the head count from the report while a column still reads", async () => {
    vi.stubGlobal(
      "fetch",
      backendWith(
        stubBackend(twoDeals, { stageTotalsRows: reportRows }),
        (url) =>
          url.searchParams.get("stage_id") === "s3"
            ? new Promise<Response>(() => {})
            : undefined,
      ),
    );
    render(<DealsScreen />);

    await waitFor(() => expect(headCount("260 deals")).toHaveLength(1));
  });

  it("states no count over a column whose first read failed", async () => {
    vi.stubGlobal(
      "fetch",
      backendWith(stubBackend(twoDeals), (url) =>
        url.searchParams.get("stage_id") === "s3"
          ? Promise.resolve(jsonResponse({ code: "internal" }, 500))
          : undefined,
      ),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("Fleet two")).toBeTruthy();
    expect(await within(column("s3")).findByRole("alert")).toBeTruthy();
    expect(
      column("s3").querySelector(".board-col-count:not([hidden])"),
    ).toBeNull();
  });

  // A failed column adds nothing to the sum, so the sum is not the whole board.
  it("says the head counts only the loaded cards while a column's read failed", async () => {
    window.location.hash = "#/deals?q=fleet";
    vi.stubGlobal(
      "fetch",
      backendWith(stubBackend(twoDeals), (url) =>
        url.searchParams.get("stage_id") === "s3"
          ? Promise.resolve(jsonResponse({ code: "internal" }, 500))
          : undefined,
      ),
    );
    render(<DealsScreen />);

    expect(await screen.findByText("1 to 2 of 2 deals loaded")).toBeTruthy();
  });
});

describe("a board column's foot", () => {
  const failedRefetch = (more: boolean) => ({
    deals: [],
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    hasNextPage: more,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    loadMoreError: new Error("The server could not answer."),
  });

  it("offers Retry for a failed read once the column has no page left", async () => {
    const read: StageColumnRead = failedRefetch(false);
    const user = userEvent.setup();
    render(<StageColumnFoot read={read} />);

    expect(screen.getByRole("alert")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(read.refetch).toHaveBeenCalledTimes(1);
  });

  it("leaves the retry to Load more while the column has a page left", () => {
    render(<StageColumnFoot read={failedRefetch(true)} />);

    expect(screen.getByRole("button", { name: "Load more" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });
});
