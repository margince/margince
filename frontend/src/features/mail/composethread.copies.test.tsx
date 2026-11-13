/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider, useT } from "../../i18n";
import { useRecentConversations, useThreadMessages } from "./composethread";

// One mail that two colleagues' mailboxes captured is one row the server
// collapsed, and the composer asks for that answer instead of grouping copies.

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mail(id: string, copyIds?: string[]): Activity {
  return {
    occurred_precision: "instant",
    offers_email: false,
    id,
    kind: "email",
    subject: "Pricing",
    direction: "inbound",
    thread_key: "pricing",
    occurred_at: "2026-09-01T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-09-01T09:00:00Z",
    updated_at: "2026-09-01T09:00:00Z",
    content_state: "available",
    ...(copyIds ? { copies: copyIds.length + 1, copy_ids: copyIds } : {}),
  };
}

const own = mail("own", ["colleague"]);

function serve(rows: Activity[]) {
  const asked: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request
          ? input
          : new Request(new URL(String(input), "https://test.local"), init);
      asked.push(new URL(request.url).searchParams);
      return new Response(
        JSON.stringify({ data: rows, page: { has_more: false } }),
        { headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  return asked;
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

it("reads the thread collapsed and keeps an anchor a copy already stands for out", async () => {
  const asked = serve([own]);
  const { result } = renderHook(() => useThreadMessages(mail("colleague")), {
    wrapper,
  });
  await waitFor(() => expect(result.current.pending).toBe(false));
  expect(asked[0]?.get("collapse_copies")).toBe("true");
  expect(result.current.messages.map((message) => message.id)).toEqual(["own"]);
});

it("reads the recent conversations collapsed and names every copy as a member", async () => {
  const asked = serve([own]);
  const { result } = renderHook(
    () => {
      const t = useT();
      return useRecentConversations("contact", "c1", true, {
        nameOf: () => undefined,
        t,
        locale: "en",
      });
    },
    { wrapper },
  );
  await waitFor(() => expect(result.current.pending).toBe(false));
  expect(asked[0]?.get("collapse_copies")).toBe("true");
  expect(result.current.conversations).toHaveLength(1);
  expect(result.current.conversations[0]?.memberIds).toEqual([
    "own",
    "colleague",
  ]);
});
