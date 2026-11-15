/** @vitest-environment happy-dom */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type ListPage, useListQuery } from "../components/listquery";
import { listOpening } from "../components/listquery.options";
import { useTerminalLeads } from "../features/leads/leadboard.queries";
import { type DealFilters, useDeals } from "../lib/deals.queries";
import { useMe } from "../lib/session";
import { cancellationClient, type HeldRead, holdReads } from "./cancellation";
import { meFixture } from "./mefixture";

// A same-user grant change keeps record caches. So a list read under the old
// reader scope must never be served under the new one, in either direction.

type Row = { id: string };
type Scope = "own" | "all";

let reads: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of reads) read.answer(page([]));
  });
  reads = [];
  vi.restoreAllMocks();
});

function page(ids: readonly string[]) {
  return {
    data: ids.map((id) => ({ id })),
    page: { has_more: false, next_cursor: null },
  };
}

const deals: DealFilters = {
  pipelineId: "pipeline",
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
};

// Each list kind through its real hook. `rows` reads the ids it shows.
const kinds = [
  {
    kind: "a generic record list (useListQuery)",
    path: "/v1/contacts",
    useRows: (): string[] =>
      useListQuery<Row>({
        key: "reader-scope-list",
        fetchPage: async (_query, _cursor, signal) => {
          const response = await fetch("https://test.local/v1/contacts", {
            signal,
          });
          return (await response.json()) as ListPage<Row>;
        },
      }).rows.map((row) => row.id),
  },
  {
    kind: "the deals table (useDeals)",
    path: "/v1/deals",
    useRows: (): string[] =>
      (useDeals(deals).data?.pages ?? []).flatMap((p) =>
        p.data.map((deal) => deal.id),
      ),
  },
  {
    kind: "a lead board terminal column (useTerminalLeads)",
    path: "/v1/leads",
    useRows: (): string[] =>
      (
        useTerminalLeads(
          listOpening(undefined, undefined),
          "disqualified",
          true,
        ).data?.pages ?? []
      ).flatMap((p) => p.data.map((lead) => lead.id)),
  },
] as const;

const directions = [
  { from: "own", to: "all" },
  { from: "all", to: "own" },
] as const satisfies readonly { from: Scope; to: Scope }[];

describe.each(kinds)("$kind", ({ path, useRows }) => {
  function signedIn(scope: Scope) {
    reads = holdReads((url) => url.pathname === path);
    const { client, wrapper } = cancellationClient();
    client.setQueryData(["me"], meFixture({ rowScope: scope }));
    return { client, wrapper };
  }

  it.each(directions)(
    "a mounted list reads again and drops the old rows when the row scope goes $from to $to",
    async ({ from, to }) => {
      const { client, wrapper } = signedIn(from);
      const view = renderHook(
        () => {
          useMe();
          return useRows();
        },
        { wrapper },
      );
      await waitFor(() => expect(reads).toHaveLength(1));
      await act(async () => reads[0]?.answer(page(["old"])));
      await waitFor(() => expect(view.result.current).toEqual(["old"]));

      await act(async () => {
        client.setQueryData(["me"], meFixture({ rowScope: to }));
      });

      await waitFor(() => expect(reads).toHaveLength(2));
      expect(view.result.current).toEqual([]);
      await act(async () => reads[1]?.answer(page(["new"])));
      await waitFor(() => expect(view.result.current).toEqual(["new"]));
    },
  );

  it.each(directions)(
    "a list opened again after the row scope went $from to $to reads its own rows",
    async ({ from, to }) => {
      const { client, wrapper } = signedIn(from);
      const first = renderHook(() => useRows(), { wrapper });
      await waitFor(() => expect(reads).toHaveLength(1));
      await act(async () => reads[0]?.answer(page(["old"])));
      await waitFor(() => expect(first.result.current).toEqual(["old"]));
      first.unmount();

      // Nothing that would drop the old entry is mounted, and the client
      // never marks anything stale: only the key keeps the old rows away.
      client.setQueryData(["me"], meFixture({ rowScope: to }));
      const again = renderHook(() => useRows(), { wrapper });

      await waitFor(() => expect(reads).toHaveLength(2));
      expect(again.result.current).toEqual([]);
      await act(async () => reads[1]?.answer(page(["new"])));
      await waitFor(() => expect(again.result.current).toEqual(["new"]));
    },
  );

  it("reads nothing before /me names the reader", async () => {
    reads = holdReads((url) => url.pathname === path);
    const { client, wrapper } = cancellationClient();
    client.removeQueries({ queryKey: ["me"] });
    vi.mocked(globalThis.fetch).mockImplementation(
      () => new Promise<Response>(() => {}),
    );
    renderHook(() => useRows(), { wrapper });

    await act(() => Promise.resolve());
    expect(reads).toHaveLength(0);
  });
});
