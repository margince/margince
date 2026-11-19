/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { useRecordCarriesTags } from "./tags.queries";

const tag = { tag_id: "t-1", name: "Champion" };

// The row folds only when every read it holds says the record has no tags.
function carries(
  carried: readonly unknown[] | undefined,
  cached?: { data: unknown[]; withheld: boolean },
): boolean {
  const client = new QueryClient();
  if (cached) client.setQueryData(["record-tags", "contact", "p-1"], cached);
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const { result } = renderHook(
    () => useRecordCarriesTags("contact", "p-1", carried),
    { wrapper },
  );
  return result.current;
}

describe("useRecordCarriesTags", () => {
  it("folds when the page and the panel both hold none", () => {
    expect(carries([], { data: [], withheld: false })).toBe(false);
    expect(carries(undefined)).toBe(false);
  });

  it("shows tags the page's own read carries before any panel read", () => {
    expect(carries([tag])).toBe(true);
  });

  it("shows tags the panel's read holds", () => {
    expect(carries([], { data: [tag], withheld: false })).toBe(true);
  });

  it("lets newer carried tags win over an older empty panel read", () => {
    expect(carries([tag], { data: [], withheld: false })).toBe(true);
  });

  it("stays shown for the record once shown, after its last tag goes", () => {
    const client = new QueryClient();
    const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result, rerender } = renderHook(
      ({ id, carried }: { id: string; carried: unknown[] }) =>
        useRecordCarriesTags("contact", id, carried),
      { wrapper, initialProps: { id: "p-1", carried: [tag] } },
    );
    rerender({ id: "p-1", carried: [] });
    expect(result.current).toBe(true);
    rerender({ id: "p-2", carried: [] });
    expect(result.current).toBe(false);
  });

  it("shows a withheld read, so the panel can say why", () => {
    expect(carries([], { data: [], withheld: true })).toBe(true);
  });
});
