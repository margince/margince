import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { jsonResponse } from "./story-utils";

/**
 * The labels an administered-list hook answers with, read in one locale off a
 * stubbed read of `rows`. The caller unstubs the globals after each case.
 */
export async function labelsIn(
  locale: Locale,
  path: string,
  rows: readonly unknown[],
  useList: () => { data?: readonly { label: string }[] },
): Promise<string[]> {
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "http://margince.test",
    );
    return url.pathname.endsWith(path)
      ? jsonResponse({ data: rows })
      : jsonResponse({ title: "Not stubbed", status: 404 }, 404);
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const { result } = renderHook(useList, { wrapper });
  await waitFor(() => expect(result.current.data).toBeDefined());
  return (result.current.data ?? []).map((row) => row.label);
}
