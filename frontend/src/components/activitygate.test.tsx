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
  return renderHook(() => useActivityGate("deal", writableTarget(record)), {
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
  it("names the role when the role may change neither the record nor an activity", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read"], deal: ["read"] } },
        { writable: false },
      ),
    ).toBe("record.roleReadsOnly");
  });

  it("names the missing action for a role that may change the record but write no activity", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read"], deal: ["read", "update"] } },
        { writable: true },
      ),
    ).toBe("record.actionRefused");
  });

  it("names the seat on a read seat, whatever the role holds", () => {
    expect(
      refusalKeyFor(
        {
          allow: { activity: ["read", "create"], deal: ["read", "update"] },
          seat: "read",
        },
        { writable: false },
      ),
    ).toBe("settings.seatRefused");
  });

  it("names the role on a row it may not file onto when the role may not change the record", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"], deal: ["read"] } },
        { writable: false },
      ),
    ).toBe("record.roleReadsOnly");
  });

  it("keeps the owner sentence for a writing role on a row it may not file onto", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"], deal: ["read", "update"] } },
        { writable: false },
      ),
    ).toBe("record.attachRefused");
  });

  it("refuses nothing to a writing role on its own row", () => {
    expect(
      refusalKeyFor(
        { allow: { activity: ["read", "create"], deal: ["read", "update"] } },
        { writable: true },
      ),
    ).toBeUndefined();
  });
});
