/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { usePipeline } from "./deals.queries";
import { pipelinesOptions, usePipelines } from "./pipelines.queries";

const client = new QueryClient({
  defaultOptions: { queries: { retry: false, staleTime: Infinity } },
});
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}
afterEach(() => {
  cleanup();
  client.clear();
  vi.restoreAllMocks();
});

it("shares raw pipelines across list and selected observers and refreshes both", async () => {
  let pipelineName = "Other";
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
    Response.json({
      data: [
        { id: "default", name: "Default", is_default: true, stages: [] },
        {
          id: "other",
          name: pipelineName,
          stages: [
            {
              id: "s",
              name: "Stored",
              name_translations: { en: "Translated" },
            },
          ],
        },
      ],
    }),
  );
  const { result, rerender } = renderHook(
    ({ id }) => ({ list: usePipelines(), selected: usePipeline(id) }),
    { initialProps: { id: "other" }, wrapper },
  );
  await waitFor(() => expect(result.current.selected.data?.id).toBe("other"));
  expect(result.current.selected.data?.stages?.[0]?.name).toBe("Translated");
  expect(client.getQueryData(pipelinesOptions.queryKey)).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        id: "other",
        stages: [expect.objectContaining({ name: "Stored" })],
      }),
    ]),
  );
  rerender({ id: "missing" });
  expect(result.current.selected.data?.id).toBe("default");
  expect(fetchMock).toHaveBeenCalledTimes(1);
  rerender({ id: "other" });
  pipelineName = "Changed";
  await client.invalidateQueries({ queryKey: ["pipelines"] });
  await waitFor(() =>
    expect(result.current.list.data?.[1]?.name).toBe("Changed"),
  );
  await waitFor(() =>
    expect(result.current.selected.data?.name).toBe("Changed"),
  );
  expect(result.current.selected.data?.stages?.[0]?.name).toBe("Translated");
  expect(fetchMock).toHaveBeenCalledTimes(2);
});

it("keeps an empty catalog usable by the list while the selected observer fails", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ data: [] }));
  const { result } = renderHook(
    () => ({ list: usePipelines(), selected: usePipeline() }),
    { wrapper },
  );
  await waitFor(() => expect(result.current.selected.isError).toBe(true));
  expect(result.current.list.data).toEqual([]);
  expect(client.getQueryState(pipelinesOptions.queryKey)?.status).toBe(
    "success",
  );
});

it("reuses a cold record's default catalog when its pipeline ID arrives", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({
      data: [
        { id: "default", name: "Default", is_default: true, stages: [] },
        { id: "other", name: "Other", is_default: false, stages: [] },
      ],
    }),
  );
  const initialProps: { id: string | undefined } = { id: undefined };
  const { result, rerender } = renderHook(({ id }) => usePipeline(id), {
    initialProps,
    wrapper,
  });
  await waitFor(() => expect(result.current.data?.id).toBe("default"));
  rerender({ id: "other" });
  await waitFor(() => expect(result.current.data?.id).toBe("other"));
  expect(fetchMock).toHaveBeenCalledTimes(1);
});
