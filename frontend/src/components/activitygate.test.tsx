// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { sessionOptions } from "../lib/session";
import { meFixture } from "../testing/mefixture";
import {
  useActivityGate,
  useWritingVerbOffered,
  writableTarget,
} from "./activitygate";

afterEach(cleanup);

function withMe(allow: Parameters<typeof meFixture>[0]) {
  const client = new QueryClient();
  client.setQueryData(sessionOptions.queryKey, meFixture(allow));
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

function offeredTo(allow: Parameters<typeof meFixture>[0]): boolean {
  return renderHook(() => useWritingVerbOffered("log_activity"), {
    wrapper: withMe(allow),
  }).result.current;
}

function refusalKeyFor(
  allow: Parameters<typeof meFixture>[0],
  record: { writable?: boolean },
) {
  return renderHook(() => useActivityGate(writableTarget(record)), {
    wrapper: withMe(allow),
  }).result.current.refusalKey;
}

describe("useWritingVerbOffered", () => {
  it("withholds a writing verb from a seat that may not create an activity", () => {
    expect(offeredTo({ allow: { activity: ["read"] } })).toBe(false);
  });

  it("offers a writing verb to a seat that may create an activity", () => {
    expect(offeredTo({ allow: { activity: ["read", "create"] } })).toBe(true);
  });
});

describe("useActivityGate", () => {
  it("names the role when the role may not create an activity", () => {
    expect(
      refusalKeyFor({ allow: { activity: ["read"] } }, { writable: true }),
    ).toBe("record.roleReadsOnly");
  });

  it("names the role on a read seat", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"] }, seat: "read" },
        { writable: false },
      ),
    ).toBe("record.roleReadsOnly");
  });

  it("keeps the owner sentence for a writing role on a row it may not file onto", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"] } },
        { writable: false },
      ),
    ).toBe("record.attachRefused");
  });

  it("refuses nothing to a writing role on its own row", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"] } },
        { writable: true },
      ),
    ).toBeUndefined();
  });
});
