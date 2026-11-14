/** @vitest-environment happy-dom */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { type DealFilters, useStageTotals } from "../../lib/deals.queries";
import {
  cancellationClient,
  type HeldRead,
  holdReads,
} from "../../testing/cancellation";
import { UNNARROWED_LIST_QUERY } from "../../testing/listquery.fixture";
import { useLeadStatusCounts, useTerminalLeads } from "./leadboard.queries";

let held: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of held)
      read.answer({ data: [], rows: [], page: { has_more: false } });
  });
  vi.restoreAllMocks();
});

const deals: DealFilters = {
  pipelineId: "pipeline",
  q: "",
  sort: "",
  includeArchived: false,
  filters: {},
};

it.each([
  {
    path: "/v1/leads",
    useRead: () =>
      useTerminalLeads(UNNARROWED_LIST_QUERY, "disqualified", true),
  },
  {
    path: "/v1/reports/leads-by-status",
    useRead: () => useLeadStatusCounts(UNNARROWED_LIST_QUERY),
  },
  { path: "/v1/reports/deals-by-stage", useRead: () => useStageTotals(deals) },
])("aborts the board read $path when it closes", async ({ path, useRead }) => {
  held = holdReads((url) => url.pathname === path);
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => {
      useRead();
    },
    { wrapper },
  );
  await waitFor(() => expect(held).toHaveLength(1));
  view.unmount();
  expect(held[0]?.request.signal.aborted).toBe(true);
  client.clear();
});

it("does not request closed terminal columns or totals withheld by a search", async () => {
  held = holdReads(() => true);
  const { client, wrapper } = cancellationClient();
  const view = renderHook(
    () => ({
      terminal: useTerminalLeads(UNNARROWED_LIST_QUERY, "disqualified", false),
      counts: useLeadStatusCounts({ ...UNNARROWED_LIST_QUERY, q: "search" }),
      totals: useStageTotals(deals, false),
    }),
    { wrapper },
  );
  await act(async () => {});
  expect(held).toHaveLength(0);
  view.unmount();
  client.clear();
});
