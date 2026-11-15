/** @vitest-environment happy-dom */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { useEntityName } from "../../components/entityref.queries";
import {
  cancellationClient,
  type HeldRead,
  holdReads,
} from "../../testing/cancellation";
import { useEmploymentPages } from "./employmentpages";

const view: components["schemas"]["Contact360"] = {
  as_of: "2026-09-01T00:00:00Z",
  sections_omitted: [],
  contact: {
    id: "contact",
    full_name: "Contact",
    source: "manual",
    captured_by: "human:sample",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  },
  employments: { data: [], page: { has_more: true, next_cursor: "older" } },
};
const relationshipPage = {
  data: [{ id: "role", company_id: "company", version: 1 }],
  page: { has_more: false },
};
let held: HeldRead[] = [];
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const read of held)
      read.answer({ data: [], page: { has_more: false } });
  });
  vi.restoreAllMocks();
});

it("aborts the relationship page and skips company enrichment after cancellation", async () => {
  held = holdReads(() => true);
  const { client, wrapper } = cancellationClient();
  const result = renderHook(() => useEmploymentPages(view), { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  expect(new URL(held[0]?.request.url ?? "http://localhost").pathname).toBe(
    "/v1/relationships",
  );
  result.unmount();
  expect(held[0]?.request.signal.aborted).toBe(true);
  await act(async () => {
    held[0]?.answer(relationshipPage);
  });
  expect(held).toHaveLength(1);
  expect(
    client
      .getQueriesData({ queryKey: ["contactEmployments"] })
      .every(([, data]) => data === undefined),
  ).toBe(true);
  client.clear();
});

it("lets company enrichment finish for another observer after the employment page closes", async () => {
  held = holdReads(() => true);
  const { client, wrapper } = cancellationClient();
  const employment = renderHook(() => useEmploymentPages(view), { wrapper });
  await waitFor(() => expect(held).toHaveLength(1));
  await act(async () => {
    held[0]?.answer(relationshipPage);
  });
  await waitFor(() => expect(held).toHaveLength(2));
  const name = renderHook(() => useEntityName("company", "company"), {
    wrapper,
  });
  employment.unmount();
  expect(held[1]?.request.signal.aborted).toBe(false);
  await act(async () => {
    held[1]?.answer({ data: [{ id: "company", display_name: "Employer" }] });
  });
  await waitFor(() => expect(name.result.current.name).toBe("Employer"));
  expect(held).toHaveLength(2);
  expect(
    client
      .getQueriesData({ queryKey: ["contactEmployments"] })
      .every(([, data]) => data === undefined),
  ).toBe(true);
  name.unmount();
  client.clear();
});

it("does not request a page when the initial employment list is complete", async () => {
  held = holdReads(() => true);
  const { client, wrapper } = cancellationClient();
  const result = renderHook(
    () =>
      useEmploymentPages({
        ...view,
        employments: { data: [], page: { has_more: false } },
      }),
    { wrapper },
  );
  await act(async () => {});
  expect(held).toHaveLength(0);
  result.unmount();
  client.clear();
});
