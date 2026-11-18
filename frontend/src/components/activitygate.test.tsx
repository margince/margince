// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { sessionOptions } from "../lib/session";
import { MOMENT_VERB_WRITES, type MomentVerbKind } from "../lib/writeverbs";
import { meFixture } from "../testing/mefixture";
import { useVerbOffered } from "./activitygate";

afterEach(cleanup);

function offeredTo(
  allow: Parameters<typeof meFixture>[0],
  kind: MomentVerbKind,
): boolean {
  const client = new QueryClient();
  client.setQueryData(sessionOptions.queryKey, meFixture(allow));
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useVerbOffered(kind), { wrapper }).result.current;
}

describe("useVerbOffered", () => {
  const kinds = Object.keys(MOMENT_VERB_WRITES) as MomentVerbKind[];

  it("withholds exactly the verbs the server classifies as writing from a seat that may not create an activity", () => {
    const withheld = kinds.filter(
      (kind) => !offeredTo({ allow: { activity: ["read"] } }, kind),
    );
    expect(withheld.sort()).toEqual(
      kinds.filter((kind) => MOMENT_VERB_WRITES[kind]).sort(),
    );
    expect(withheld).toContain("log_activity");
    expect(withheld).not.toContain("open_record");
  });

  it("offers every verb to a seat that may create an activity", () => {
    const grant = { allow: { activity: ["read", "create"] as const } };
    expect(kinds.every((kind) => offeredTo(grant, kind))).toBe(true);
  });
});
