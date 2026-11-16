/** @vitest-environment happy-dom */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  cancellationClient,
  type HeldRead,
  holdReads,
} from "../testing/cancellation";
import { meFixture } from "../testing/mefixture";
import { type DealFilters, useStageTotals } from "./deals.queries";
import { useMe } from "./session";

const board: DealFilters = {
  pipelineId: "pipeline",
  q: "",
  sort: "name",
  includeArchived: false,
  filters: {},
};

function report(deals: number) {
  return { rows: [{ stage_id: "stage", deals }], base_currency: "EUR" };
}

let reads: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of reads) read.answer(report(0));
  });
  vi.restoreAllMocks();
});

function holdTotals() {
  reads = holdReads((url) => url.pathname === "/v1/reports/deals-by-stage");
}

type Props = { f: DealFilters; enabled: boolean };

async function boardWithTotals(f: DealFilters = board) {
  holdTotals();
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    ({ f, enabled }: Props) => useStageTotals(f, enabled),
    { wrapper, initialProps: { f, enabled: true } },
  );
  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => reads[0]?.answer(report(7)));
  await waitFor(() =>
    expect(view.result.current.data?.get("stage")?.count).toBe(7),
  );
  return { client, view };
}

it("reuses fresh totals after a table sort and sends the same body", async () => {
  const { client, view } = await boardWithTotals();
  expect(await reads[0]?.request.clone().json()).toEqual({
    group_by: ["stage_id"],
    aggregates: [
      { fn: "count", as: "deals" },
      { fn: "sum", field: "amount_base_minor", as: "base_minor" },
      { fn: "sum", field: "weighted_base_minor", as: "weighted_base_minor" },
      { fn: "count", field: "amount_base_minor", as: "priced_deals" },
    ],
    filters: { pipeline_id: "pipeline" },
  });
  const sorted = { ...board, sort: "-amount_minor" };
  view.rerender({ f: sorted, enabled: false });
  view.rerender({ f: sorted, enabled: true });
  await act(async () => {});
  expect(view.result.current.data?.get("stage")?.count).toBe(7);
  expect(reads).toHaveLength(1);
  view.unmount();
  client.clear();
});

it.each<Partial<DealFilters>>([
  { pipelineId: "other" },
  { filters: { owner_id: "owner" } },
  { filters: { company_id: "company" } },
  { filters: { status: "won" } },
  { filters: { partner_company_id: "partner" } },
  { filters: { stalled: "true" } },
  { filters: { partner_sourced: "true" } },
])("reads again when the population changes: %j", async (change) => {
  const { client, view } = await boardWithTotals();
  view.rerender({ f: { ...board, ...change }, enabled: true });
  await waitFor(() => expect(reads).toHaveLength(2));
  expect(view.result.current.data).toBeUndefined();
  const body = await reads[1]?.request.clone().json();
  expect(body.filters).not.toEqual({ pipeline_id: "pipeline" });
  view.unmount();
  client.clear();
});

it.each([
  { ...board, q: "search" },
  { ...board, includeArchived: true },
  { ...board, filters: { stage_id: "stage" } },
  { ...board, filters: { owner_team_id: "team" } },
  { ...board, filters: { unassigned: "true" } },
  { ...board, filters: { tag_id: "tag" } },
])("never shows a cached total for withheld filters %j", async (f) => {
  const { client, view } = await boardWithTotals();
  view.rerender({ f, enabled: true });
  expect(view.result.current.data).toBeUndefined();
  expect(view.result.current.fetchStatus).toBe("idle");
  expect(reads).toHaveLength(1);
  view.unmount();
  client.clear();
});

it("refreshes totals invalidated while the table shows", async () => {
  const { client, view } = await boardWithTotals();
  view.rerender({ f: board, enabled: false });
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["deals"] });
  });
  expect(reads).toHaveLength(1);
  view.rerender({ f: board, enabled: true });
  await waitFor(() => expect(reads).toHaveLength(2));
  await act(async () => reads[1]?.answer(report(8)));
  await waitFor(() =>
    expect(view.result.current.data?.get("stage")?.count).toBe(8),
  );
  view.unmount();
  client.clear();
});

it("does not hand one user's totals to the next user", async () => {
  holdTotals();
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => {
      useMe();
      return useStageTotals(board);
    },
    { wrapper },
  );
  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => reads[0]?.answer(report(7)));
  await waitFor(() =>
    expect(view.result.current.data?.get("stage")?.count).toBe(7),
  );
  const me = meFixture({ rowScope: "all" });
  await act(async () => {
    client.setQueryData(["me"], {
      ...me,
      user: { ...me.user, id: "another-user" },
    });
  });
  await waitFor(() => expect(reads).toHaveLength(2));
  expect(view.result.current.data).toBeUndefined();
  await act(async () => reads[1]?.answer(report(2)));
  await waitFor(() =>
    expect(view.result.current.data?.get("stage")?.count).toBe(2),
  );
  view.unmount();
  client.clear();
});

it.each([
  { from: "own", to: "all" },
  { from: "all", to: "own" },
] as const)(
  "reads again when the same user's row scope goes from $from to $to",
  async ({ from, to }) => {
    holdTotals();
    const { client, wrapper } = cancellationClient();
    client.setQueryData(["me"], meFixture({ rowScope: from }));
    const view = renderHook(
      () => {
        useMe();
        return useStageTotals(board);
      },
      { wrapper },
    );
    await waitFor(() => expect(reads).toHaveLength(1));
    await act(async () => reads[0]?.answer(report(7)));
    await waitFor(() =>
      expect(view.result.current.data?.get("stage")?.count).toBe(7),
    );
    await act(async () => {
      client.setQueryData(["me"], meFixture({ rowScope: to }));
    });
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(view.result.current.data).toBeUndefined();
    await act(async () => reads[1]?.answer(report(3)));
    await waitFor(() =>
      expect(view.result.current.data?.get("stage")?.count).toBe(3),
    );
    view.unmount();
    client.clear();
  },
);

it("drops a cached refusal when the same user's grants change", async () => {
  holdTotals();
  const { client, wrapper } = cancellationClient();
  client.setQueryData(["me"], meFixture({ rowScope: "own" }));
  const view = renderHook(() => useStageTotals(board), { wrapper });
  await waitFor(() => expect(reads).toHaveLength(1));
  await act(async () => reads[0]?.answer({ detail: "refused" }, 403));
  await waitFor(() => expect(view.result.current.data).toBeNull());
  await act(async () => {
    client.setQueryData(["me"], meFixture({ rowScope: "all" }));
  });
  await waitFor(() => expect(reads).toHaveLength(2));
  expect(view.result.current.data).toBeUndefined();
  view.unmount();
  client.clear();
});
