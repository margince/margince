/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type DealFilters,
  STAGE_COLUMN_LIMIT,
  useStageColumns,
} from "./deals.queries";

afterEach(() => {
  vi.unstubAllGlobals();
});

// A keyset server over deal ids per stage. The cursor is the next page's offset,
// and a stage asked for answers only its own deals.
function stubServer(byStage: Record<string, string[]>) {
  const asked: URL[] = [];
  const failing = new Set<string>();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      if (!url.pathname.endsWith("/deals")) {
        return Response.json({});
      }
      asked.push(url);
      const stage = url.searchParams.get("stage_id") ?? "";
      const from = Number(url.searchParams.get("cursor") ?? 0);
      if (failing.has(`${stage}@${from}`)) {
        return Response.json({ code: "internal" }, { status: 500 });
      }
      const limit = Number(url.searchParams.get("limit"));
      const ids = stage
        ? (byStage[stage] ?? [])
        : Object.values(byStage).flat();
      const more = from + limit < ids.length;
      return Response.json({
        data: ids.slice(from, from + limit).map((id) => ({
          id,
          name: id,
          stage_id: stage,
          version: 1,
        })),
        page: {
          next_cursor: more ? String(from + limit) : null,
          has_more: more,
        },
      });
    }),
  );
  return { asked, failing };
}

const ids = (prefix: string, count: number) =>
  Array.from({ length: count }, (_, index) => `${prefix}-${index}`);

const noDials: DealFilters = {
  pipelineId: "pl",
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
};

function renderColumns(f: DealFilters, stageIds: string[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const view = renderHook(() => useStageColumns(f, stageIds), { wrapper });
  return { ...view, client };
}

const column = (
  view: ReturnType<typeof renderColumns>["result"],
  stageId: string,
) => {
  const read = view.current.columns.get(stageId);
  if (!read) throw new Error(`no column read for ${stageId}`);
  return read;
};

describe("useStageColumns", () => {
  it("reads each stage on its own, a column's page at a time, under the board's dials", async () => {
    const { asked } = stubServer({ s1: ["a"], s2: ["b"] });
    const { result } = renderColumns(
      {
        pipelineId: "pl",
        q: "fleet",
        sort: "-amount_minor",
        includeArchived: false,
        filters: {
          owner_id: "u-1",
          tag_id: "t-1",
          stalled: "true",
          close_day: "2026-01-01..2026-03-31",
        },
      },
      ["s1", "s2"],
    );
    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(asked.map((url) => url.searchParams.get("stage_id")).sort()).toEqual(
      ["s1", "s2"],
    );
    for (const url of asked) {
      expect(url.searchParams.get("limit")).toBe(String(STAGE_COLUMN_LIMIT));
      expect(url.searchParams.get("pipeline_id")).toBe("pl");
      expect(url.searchParams.get("q")).toBe("fleet");
      expect(url.searchParams.get("sort")).toBe("-amount_minor");
      expect(url.searchParams.get("owner_id")).toBe("u-1");
      expect(url.searchParams.getAll("tag_id")).toEqual(["t-1"]);
      expect(url.searchParams.get("stalled")).toBe("true");
      expect(url.searchParams.get("close_from")).toBe("2026-01-01");
    }
  });

  // A stage whose deals all sort after the busy stage's first 25 still draws
  // them, because each column reads its own first page.
  it("draws a stage whose deals sit past the busy stage's first page", async () => {
    stubServer({ s1: ids("busy", 120), s3: ["late"] });
    const { result } = renderColumns(noDials, ["s1", "s2", "s3"]);
    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(column(result, "s3").deals.map((deal) => deal.id)).toEqual(["late"]);
    expect(column(result, "s2").deals).toEqual([]);
    expect(column(result, "s2").hasNextPage).toBe(false);
  });

  it("loads one column's next page and leaves the others where they stand", async () => {
    const { asked } = stubServer({ s1: ids("s1", 30), s2: ids("s2", 30) });
    const { result } = renderColumns(noDials, ["s1", "s2"]);
    await waitFor(() => expect(result.current.isPending).toBe(false));
    expect(column(result, "s1").hasNextPage).toBe(true);
    const before = asked.length;

    act(() => {
      column(result, "s1").fetchNextPage();
    });

    await waitFor(() => expect(column(result, "s1").deals).toHaveLength(30));
    const loadMore = asked.slice(before);
    expect(
      loadMore.map((url) => [
        url.searchParams.get("stage_id"),
        url.searchParams.get("cursor"),
      ]),
    ).toEqual([["s1", "25"]]);
    expect(column(result, "s1").hasNextPage).toBe(false);
    expect(column(result, "s2").deals).toHaveLength(25);
  });

  // A deal that joins the top of a column pushes its old last row onto the
  // next page. The refetch walks again from the top, so that row is neither
  // lost between the pages nor drawn twice.
  it("refetches a column from its first page, so a deal joining it opens no gap", async () => {
    const byStage = { s1: ids("s1", 30) };
    stubServer(byStage);
    const { result, client } = renderColumns(noDials, ["s1"]);
    await waitFor(() => expect(result.current.isPending).toBe(false));
    act(() => {
      column(result, "s1").fetchNextPage();
    });
    await waitFor(() => expect(column(result, "s1").deals).toHaveLength(30));

    byStage.s1 = ["moved-in", ...byStage.s1];
    await act(() => client.invalidateQueries({ queryKey: ["deals"] }));

    await waitFor(() => expect(column(result, "s1").deals).toHaveLength(31));
    const drawn = column(result, "s1").deals.map((deal) => deal.id);
    expect(new Set(drawn).size).toBe(31);
    expect(drawn).toEqual(byStage.s1);
  });

  it("keeps the other columns when one column's first page fails", async () => {
    const { failing } = stubServer({ s1: ["a"], s2: ["b"] });
    failing.add("s2@0");
    const { result } = renderColumns(noDials, ["s1", "s2"]);
    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(column(result, "s1").deals.map((deal) => deal.id)).toEqual(["a"]);
    expect(column(result, "s2").isError).toBe(true);
    expect(column(result, "s2").hasNextPage).toBe(false);
  });

  it("keeps a column's cards when a later page fails, and its button retries that page", async () => {
    const { asked, failing } = stubServer({ s1: ids("s1", 30) });
    failing.add("s1@25");
    const { result } = renderColumns(noDials, ["s1"]);
    await waitFor(() => expect(result.current.isPending).toBe(false));
    act(() => {
      column(result, "s1").fetchNextPage();
    });
    await waitFor(() =>
      expect(column(result, "s1").loadMoreError).toBeTruthy(),
    );
    expect(column(result, "s1").deals).toHaveLength(25);
    expect(column(result, "s1").isError).toBe(false);
    expect(column(result, "s1").hasNextPage).toBe(true);

    failing.clear();
    const before = asked.length;
    act(() => {
      column(result, "s1").fetchNextPage();
    });
    await waitFor(() => expect(column(result, "s1").deals).toHaveLength(30));
    expect(
      asked.slice(before).map((url) => url.searchParams.get("cursor")),
    ).toEqual(["25"]);
  });

  // Under a stage filter the other columns hold nothing, which is the truth.
  it("reads only the filtered stage", async () => {
    const { asked } = stubServer({ s1: ["a"], s2: ["b"] });
    const { result } = renderColumns(
      { ...noDials, filters: { stage_id: "s2" } },
      ["s1", "s2"],
    );
    await waitFor(() => expect(result.current.isPending).toBe(false));

    expect(asked.map((url) => url.searchParams.get("stage_id"))).toEqual([
      "s2",
    ]);
    expect(result.current.columns.has("s1")).toBe(false);
    expect(result.current.deals.map((deal) => deal.id)).toEqual(["b"]);
  });

  it("reads nothing for a pipeline with no stages", async () => {
    const { asked } = stubServer({});
    const { result } = renderColumns(noDials, []);

    expect(result.current.isPending).toBe(false);
    expect(result.current.deals).toEqual([]);
    expect(asked).toEqual([]);
  });
});
