/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { useOwnerChips } from "./ownerdials";

// The Owner dial lists colleagues and the Team dial lists teams. A team owns
// no record, so a team name among the owners read as a missing colleague. The
// team options once came from the viewer's own memberships, which left every
// other team unreachable.

const VIEWER = meFixture().user.id;

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const COLLEAGUES = [
  { id: VIEWER, display_name: "Viewer Self" },
  { id: "u-zed", display_name: "Zed Ritter" },
  { id: "u-anna", display_name: "Anna Becker" },
  { id: "u-bot", display_name: "Filing Agent", is_agent: true },
];

/**
 * `/me` on no team at all, a user roster, and a team roster walked page by
 * page. `lastTeamPage` past every index is a server that never stops offering
 * another, which makes the walk stop at its bound and report the list partial.
 */
function stub(lastTeamPage: number) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/me")) {
        return json({ ...meFixture(), teams: [] });
      }
      if (url.pathname.endsWith("/users")) {
        return json({ data: COLLEAGUES, page: { next_cursor: null } });
      }
      if (url.pathname.endsWith("/users/names")) {
        return json({ data: [] });
      }
      const cursor = url.searchParams.get("cursor");
      const index = cursor ? Number(cursor) : 0;
      const last = index >= lastTeamPage;
      return json({
        data: [
          index === 0
            ? { id: "tm-west", name: "Region West" }
            : { id: `tm-${index}`, name: `Region ${index}` },
        ],
        page: { next_cursor: last ? null : String(index + 1), has_more: !last },
      });
    }),
  );
}

function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

function dial(chips: ReturnType<typeof useOwnerChips>, key: string) {
  return chips.find((chip) => chip.key === key);
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
  globalThis.location.hash = "#/leads";
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the Owner dial", () => {
  it("offers you, then every colleague by name, then the unowned queue, and no team", async () => {
    stub(0);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    await waitFor(() =>
      expect(dial(result.current, "owner")?.options.length).toBe(4),
    );
    // The agent seat is left out: the server refuses it as an owner.
    expect(dial(result.current, "owner")?.options).toEqual([
      { value: `owner_id:${VIEWER}`, label: en["list.filterOwnerMe"] },
      { value: "owner_id:u-anna", label: "Anna Becker" },
      { value: "owner_id:u-zed", label: "Zed Ritter" },
      { value: "unassigned:true", label: en["list.filterOwnerUnassigned"] },
    ]);
    expect(dial(result.current, "owner")?.filterable).toBe(true);
  });

  it("keeps an applied owner the roster does not list, so the filter can be cleared", async () => {
    globalThis.location.hash = "#/leads?owner_id=u-gone";
    stub(0);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    await waitFor(() =>
      expect(
        dial(result.current, "owner")?.options.some(
          (option) => option.value === "owner_id:u-gone",
        ),
      ).toBe(true),
    );
  });
});

describe("the Team dial", () => {
  it("lists every team in the workspace, though the viewer is on none", async () => {
    stub(2);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    await waitFor(() =>
      expect(dial(result.current, "owner_team")?.options.length).toBe(3),
    );
    expect(
      dial(result.current, "owner_team")?.options.map((option) => option.label),
    ).toEqual(["Region 1", "Region 2", "Region West"]);
  });

  it("and the Owner dial each clear the other's parameters", async () => {
    stub(0);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    await waitFor(() =>
      expect(dial(result.current, "owner_team")).toBeDefined(),
    );
    expect(dial(result.current, "owner_team")?.excludes).toEqual([
      "owner_id",
      "unassigned",
    ]);
    expect(dial(result.current, "owner")?.excludes).toEqual(["owner_team_id"]);
  });

  it("keeps an applied team the bounded walk never reached, and says the name did not load", async () => {
    globalThis.location.hash = "#/leads?owner_team_id=tm-far";
    stub(Number.POSITIVE_INFINITY);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    const labelFor = (value: string) =>
      dial(result.current, "owner_team")?.options.find(
        (option) => option.value === value,
      )?.label;
    await waitFor(() =>
      expect(labelFor("owner_team_id:tm-far")).toBe(en["ref.nameLoadFailed"]),
    );
    expect(labelFor("owner_team_id:tm-west")).toBe("Region West");
  });

  it("keeps an applied team the finished roster does not list, as unavailable", async () => {
    globalThis.location.hash = "#/leads?owner_team_id=tm-archived";
    stub(0);
    const { result } = renderHook(() => useOwnerChips(), { wrapper });

    await waitFor(() =>
      expect(
        dial(result.current, "owner_team")?.options.some(
          (option) => option.label === "Region West",
        ),
      ).toBe(true),
    );
    // `/teams` leaves archived teams out, but the request is still narrowed by
    // this one. Without its option the dial would read "Any team" over a
    // narrowed list, with no way to clear it.
    expect(dial(result.current, "owner_team")?.options).toEqual([
      { value: "owner_team_id:tm-west", label: "Region West" },
      {
        value: "owner_team_id:tm-archived",
        label: en["list.teamUnavailable"],
      },
    ]);
  });
});
