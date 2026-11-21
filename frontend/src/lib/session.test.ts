/** @vitest-environment happy-dom */
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { act, cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  AuthProbeError,
  type AuthProbeKind,
  resetToSignedOut,
  sessionOptions,
  sessionRefusalWatch,
  useMe,
} from "./session";
import { STORAGE_KEYS } from "./storage";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  // Unconditionally, not at the end of the case that installed a spy. A case
  // that fails its assertion never reaches its own teardown. A leaked console
  // spy then silences the rest of the file, which looks like a suite that passes.
  vi.restoreAllMocks();
});

describe("signing out", () => {
  afterEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it("forgets what the member left in storage and keeps what no one else inherits", async () => {
    const declined = `${STORAGE_KEYS.platformDeclined.prefix}account-1`;
    localStorage.setItem(STORAGE_KEYS.faultsSeen.name, "run-1");
    sessionStorage.setItem(STORAGE_KEYS.oauthAttempt.name, "gmail");
    localStorage.setItem(STORAGE_KEYS.theme.name, "dark");
    localStorage.setItem(declined, "1");

    await resetToSignedOut(new QueryClient());

    expect(localStorage.getItem(STORAGE_KEYS.faultsSeen.name)).toBeNull();
    expect(sessionStorage.getItem(STORAGE_KEYS.oauthAttempt.name)).toBeNull();
    expect(localStorage.getItem(STORAGE_KEYS.theme.name)).toBe("dark");
    expect(localStorage.getItem(declined)).toBe("1");
  });
});

describe("a different seat signing in", () => {
  type Me = components["schemas"]["MeResponse"];
  const seat = (id: string, allow: GrantSpec = { contact: ["read"] }): Me => {
    const me = meFixture({ allow });
    return { ...me, user: { ...me.user, id } };
  };

  function signedIn(client: QueryClient, me: Me) {
    client.setQueryData(["me"], me);
    const wrapper = ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client }, children);
    return renderHook(() => useMe(false), { wrapper });
  }

  it("drops record caches the previous seat read, through a signed-out gap", () => {
    const client = new QueryClient();
    const view = signedIn(client, seat("seat-a"));
    client.setQueryData(["contact", "name", "c1"], "Private name");
    act(() => client.setQueryData(["me"], undefined));
    view.rerender();
    expect(client.getQueryData(["contact", "name", "c1"])).toBe("Private name");
    act(() => client.setQueryData(["me"], seat("seat-b")));
    view.rerender();
    expect(client.getQueryData(["contact", "name", "c1"])).toBeUndefined();
    expect(client.getQueryData(["me"])).toEqual(seat("seat-b"));
  });

  it("drops the previous seat's own account reads, such as its signed-in devices", () => {
    const client = new QueryClient();
    const view = signedIn(client, seat("seat-a"));
    client.setQueryData(["me", "sessions"], [{ id: "s1" }]);
    act(() => client.setQueryData(["me"], undefined));
    view.rerender();
    act(() => client.setQueryData(["me"], seat("seat-b")));
    view.rerender();
    expect(client.getQueryData(["me", "sessions"])).toBeUndefined();
  });

  it("keeps record caches when the same seat's grants change", () => {
    const client = new QueryClient();
    const view = signedIn(client, seat("seat-a"));
    client.setQueryData(["contact", "name", "c1"], "Own name");
    act(() => client.setQueryData(["me"], seat("seat-a", { deal: ["read"] })));
    view.rerender();
    expect(client.getQueryData(["contact", "name", "c1"])).toBe("Own name");
  });
});

// A refusal elsewhere re-asks /me, unless the probe's own refusal already says it.
describe("a refusal elsewhere", () => {
  async function reAsks(probe: AuthProbeKind, status: number) {
    const client = new QueryClient();
    const readSession = vi.fn(async () => {
      throw new AuthProbeError(probe, "refused");
    });
    renderHook(() => useQuery({ ...sessionOptions, queryFn: readSession }), {
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(QueryClientProvider, { client }, children),
    });
    await vi.waitFor(() =>
      expect(client.getQueryState(sessionOptions.queryKey)?.status).toBe(
        "error",
      ),
    );
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ status }, { status })),
    );
    const watch = sessionRefusalWatch(client);
    api.use(watch);
    try {
      await act(async () => {
        await api.GET("/pipelines");
      });
    } finally {
      api.eject(watch);
      vi.unstubAllGlobals();
    }
    await vi.waitFor(() => expect(client.isFetching()).toBe(0));
    client.clear();
    return readSession.mock.calls.length > 1;
  }

  it("leaves a probe that already sent the reader to sign-in alone", async () => {
    expect(await reAsks("unauthorized", 401)).toBe(false);
  });

  it("leaves a lock alone for the 403 it already explains", async () => {
    expect(await reAsks("must-enrol-mfa", 403)).toBe(false);
  });

  it("re-asks a locked session that a 401 says has lapsed", async () => {
    expect(await reAsks("must-change-password", 401)).toBe(true);
  });

  it("re-asks a probe that only failed to connect", async () => {
    expect(await reAsks("connection", 401)).toBe(true);
  });
});
