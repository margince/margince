/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { useEmploymentPages } from "./employmentpages";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it.each([true, false])(
  "loads all roles when company visibility is %s",
  async (visible) => {
    const cursors: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = new URL(new Request(input, init).url);
        if (url.pathname.endsWith("/relationships")) {
          cursors.push(url.searchParams.get("cursor") ?? "");
          return Response.json({
            data: [
              {
                id: "older-role",
                company_id: "company",
                role: "Engineer",
                employment_status: "former",
                version: 4,
              },
            ],
            page: { has_more: false },
          });
        }
        return visible
          ? Response.json({ id: "company", display_name: "Earlier Company" })
          : Response.json({ code: "not_found" }, { status: 404 });
      }),
    );
    const view: components["schemas"]["Contact360"] = {
      as_of: "2026-09-01T00:00:00Z",
      sections_omitted: [],
      contact: {
        id: "contact",
        full_name: "Sample Contact",
        source: "manual",
        captured_by: "human:sample",
        created_at: "2026-09-01T00:00:00Z",
        updated_at: "2026-09-01T00:00:00Z",
      },
      employments: {
        data: [],
        page: { has_more: true, next_cursor: "older-page" },
      },
    };
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const { result } = renderHook(() => useEmploymentPages(view), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(cursors).toEqual(["older-page"]);
    expect(result.current.data?.pages[0]?.roles[0]).toMatchObject({
      relationship_id: "older-role",
      company_name: visible ? "Earlier Company" : null,
      employment_status: "former",
      version: 4,
    });
    expect(result.current.hasNextPage).toBe(false);
  },
);
