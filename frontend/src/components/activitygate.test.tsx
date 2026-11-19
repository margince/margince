// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { sessionOptions } from "../lib/session";
import { meFixture } from "../testing/mefixture";
import { useWritingVerbOffered } from "./activitygate";

afterEach(cleanup);

function offeredTo(allow: Parameters<typeof meFixture>[0]): boolean {
  const client = new QueryClient();
  client.setQueryData(sessionOptions.queryKey, meFixture(allow));
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useWritingVerbOffered("log_activity"), { wrapper })
    .result.current;
}

describe("useWritingVerbOffered", () => {
  it("withholds a writing verb from a seat that may not create an activity", () => {
    expect(offeredTo({ allow: { activity: ["read"] } })).toBe(false);
  });

  it("offers a writing verb to a seat that may create an activity", () => {
    expect(offeredTo({ allow: { activity: ["read", "create"] } })).toBe(true);
  });
});
