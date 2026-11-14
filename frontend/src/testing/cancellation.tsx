import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { meFixture } from "./mefixture";

export function cancellationClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(["me"], meFixture({ rowScope: "all" }));
  function wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{children}</LocaleProvider>
      </QueryClientProvider>
    );
  }
  return { client, wrapper };
}

export type HeldRead = {
  request: Request;
  answer: (body: unknown, status?: number) => void;
};

// Late answers after abort let the tests check cache rollback.
export function holdReads(matches: (url: URL) => boolean) {
  const reads: HeldRead[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation((input, init) => {
    const request = new Request(input, init);
    const url = new URL(request.url);
    if (matches(url)) {
      return new Promise<Response>((resolve) => {
        reads.push({
          request,
          answer: (body, status = 200) =>
            resolve(Response.json(body, { status })),
        });
      });
    }
    const body = url.pathname.endsWith("/me")
      ? meFixture({ rowScope: "all" })
      : { data: [], page: { has_more: false, next_cursor: null }, rows: [] };
    return Promise.resolve(Response.json(body));
  });
  return reads;
}
