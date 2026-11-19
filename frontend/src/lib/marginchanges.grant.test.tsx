/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { expect, it } from "vitest";
import {
  pendingChangesQuery,
  usePendingMarginceChanges,
} from "./marginchanges";

const SUMMARY = {
  as_of: "2026-11-19T08:00:00Z",
  deals: 1,
  closed: 0,
  redated: 0,
  revalued: 0,
  probabilities: 0,
  moved: 0,
  base_currency: "EUR",
  bundles: [],
  truncated: false,
};

it("stops offering what it cached once the grant is revoked while mounted", async () => {
  const client = new QueryClient();
  client.setQueryData(pendingChangesQuery().queryKey, SUMMARY);
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  const view = renderHook(({ granted }) => usePendingMarginceChanges(granted), {
    wrapper,
    initialProps: { granted: true },
  });
  await waitFor(() => expect(view.result.current.data).toEqual(SUMMARY));
  view.rerender({ granted: false });
  expect(view.result.current.data).toBeUndefined();
});
