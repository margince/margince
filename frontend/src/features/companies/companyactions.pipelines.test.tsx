/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { usePipelines } from "../../lib/pipelines.queries";
import { stub, view } from "./company360.testkit";
import { NewDealAction } from "./companyactions";

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
  vi.unstubAllGlobals();
});

it("reuses the pipeline catalog for a company's New deal action", async () => {
  const fetched = stub(view());
  const { result } = renderHook(() => usePipelines(), { wrapper });
  await waitFor(() => expect(result.current.data?.[0]?.id).toBe("pl-1"));
  render(<NewDealAction companyId="o-1" companyName="Example" />, { wrapper });
  expect(await screen.findByRole("button", { name: "New deal" })).toBeTruthy();
  expect(fetched.filter((url) => url === "/v1/pipelines")).toHaveLength(1);
});
