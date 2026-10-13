/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { useCachedRecordName } from "./recordidentity";

// The seed a record page draws its heading from before its own read returns.
// Seeded through the cache shape a list actually writes — `pages[].data` — so a
// rename of that field fails here rather than silently answering null, which
// reads exactly like "the open did not come from a list".

function withCache(seed: (client: QueryClient) => void) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  seed(client);
  return ({ children }: Readonly<{ children: ReactNode }>) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

// One list page holding the row, which is the open PERF-1 exercises: a click
// from a list the client has already drawn.
const onePage = (client: QueryClient) =>
  client.setQueryData(["contacts", { sort: "-created_at" }], {
    pages: [{ data: [{ id: "c-1", full_name: "Anna Weber" }] }],
  });

describe("useCachedRecordName", () => {
  it("answers the name the list already holds for that id", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-1"),
      {
        wrapper: withCache(onePage),
      },
    );
    expect(result.current).toBe("Anna Weber");
  });

  it("answers null for a record no cached list page holds", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-absent"),
      { wrapper: withCache(onePage) },
    );
    // Null rather than a guess: a pasted address or a reload has no list behind
    // it, and the caller shows what it showed before.
    expect(result.current).toBe(null);
  });

  it("answers null when no list has been drawn at all", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-1"),
      {
        wrapper: withCache(() => {}),
      },
    );
    expect(result.current).toBe(null);
  });

  // The row can be on any page of any cached query for that list: the key
  // carries the sort and filters, so a record opened from a filtered or
  // scrolled list is found on a different entry than the first.
  it("finds the row on a later page of a differently-filtered list", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-9"),
      {
        wrapper: withCache((client) => {
          onePage(client);
          client.setQueryData(["contacts", { sort: "full_name", q: "web" }], {
            pages: [
              { data: [{ id: "c-4", full_name: "Bo Lind" }] },
              { data: [{ id: "c-9", full_name: "Zoe Webb" }] },
            ],
          });
        }),
      },
    );
    expect(result.current).toBe("Zoe Webb");
  });

  // A row carrying no name is not a name. Returning "" would put an empty
  // heading on screen, which is worse than the placeholder it replaced.
  it("skips a cached row that carries no name", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-2"),
      {
        wrapper: withCache((client) =>
          client.setQueryData(["contacts", {}], {
            pages: [{ data: [{ id: "c-2" }] }],
          }),
        ),
      },
    );
    expect(result.current).toBe(null);
  });

  // Another record type's list is not this one's: the key is what scopes the
  // search, so a matching id under a different list must not answer.
  it("does not read a different list's cache", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-1"),
      {
        wrapper: withCache((client) =>
          client.setQueryData(["companies", {}], {
            pages: [{ data: [{ id: "c-1", full_name: "Weber GmbH" }] }],
          }),
        ),
      },
    );
    expect(result.current).toBe(null);
  });

  // Whitespace is truthy. A row holding only spaces would put an empty heading
  // where the placeholder belongs, which looks like a broken page rather than a
  // loading one.
  it("does not seed a heading from a name that is only whitespace", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-3"),
      {
        wrapper: withCache((client) =>
          client.setQueryData(["contacts", {}], {
            pages: [{ data: [{ id: "c-3", full_name: "   " }] }],
          }),
        ),
      },
    );
    expect(result.current).toBe(null);
  });

  it("trims the name it does seed", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-4"),
      {
        wrapper: withCache((client) =>
          client.setQueryData(["contacts", {}], {
            pages: [{ data: [{ id: "c-4", full_name: "  Anna Weber  " }] }],
          }),
        ),
      },
    );
    expect(result.current).toBe("Anna Weber");
  });

  // Two cached lists can hold the same record under different names — one drawn
  // before a rename. Neither is known to be current, and the read that will say
  // is already in flight, so the seed is given up: a heading that flashes the
  // wrong name is worse than one that arrives a moment later.
  it("seeds nothing when two cached lists disagree about the name", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-5"),
      {
        wrapper: withCache((client) => {
          client.setQueryData(["contacts", { sort: "full_name" }], {
            pages: [{ data: [{ id: "c-5", full_name: "Anna Weber" }] }],
          });
          client.setQueryData(["contacts", { sort: "-created_at" }], {
            pages: [{ data: [{ id: "c-5", full_name: "Anna Weber-Lind" }] }],
          });
        }),
      },
    );
    expect(result.current).toBe(null);
  });

  // Agreement across entries is not a disagreement: the same name in two cached
  // lists is the ordinary case, and giving up there would seed almost nothing.
  it("still seeds when two cached lists agree", () => {
    const { result } = renderHook(
      () => useCachedRecordName("contacts", "c-6"),
      {
        wrapper: withCache((client) => {
          client.setQueryData(["contacts", { sort: "full_name" }], {
            pages: [{ data: [{ id: "c-6", full_name: "Bo Lind" }] }],
          });
          client.setQueryData(["contacts", { q: "lind" }], {
            pages: [{ data: [{ id: "c-6", full_name: "Bo Lind" }] }],
          });
        }),
      },
    );
    expect(result.current).toBe("Bo Lind");
  });
});
