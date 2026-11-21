/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EMPTY_DRAFT } from "../onboarding";
import { onboardingProgressOptions } from "../onboarding-progress";
import { useWizardStatePersist } from "./use-wizard-state";

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The row behind GET and PUT /onboarding/state, refusing a write whose
// expected_version is not the row's own, as the server does.
function stubServer(version: number) {
  const row = { path: "member", step: "voice", version };
  const reads: number[] = [];
  const writes: number[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (request.method === "GET") {
        reads.push(row.version);
        return json(row);
      }
      const body: { expected_version: number } = await request.clone().json();
      writes.push(body.expected_version);
      if (body.expected_version !== row.version) {
        return json({ code: "version_skew" }, 409);
      }
      row.version += 1;
      return json({ ...row, ...body });
    }),
  );
  return { row, reads, writes };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useWizardStatePersist", () => {
  it("reads the row again after a refused write and writes on its new version", async () => {
    const server = stubServer(3);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    await client.fetchQuery(onboardingProgressOptions);
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useWizardStatePersist(), { wrapper });
    // Another tab writes after this one cached the row.
    server.row.version = 4;
    const write = { step: "voice" as const, values: EMPTY_DRAFT.values };

    expect(await result.current.persist(write)).toBe(false);
    expect(server.reads).toEqual([3]);

    expect(await result.current.persist(write)).toBe(true);
    expect(server.reads).toEqual([3, 4]);
    expect(server.writes).toEqual([3, 4]);
  });
});
