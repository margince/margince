/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { memoryStorage } from "../testing/appharness";
import { Shell } from "./shell";
import { ignoreSearch, newClient, renderWith } from "./testing/shellharness";

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
  window.localStorage.clear();
});

// Sign-out is reached from the account menu in the top bar. What the menu does
// with focus and layers is account.test.tsx's; what is proved here is that the
// shell's copy of it actually ends the session — the mutation, the cache, and
// the gate that follows them. Driven through the whole SHELL because the menu is
// only reachable through the chrome that mounts it: a rail rendered on its own
// has carried no account affordance since the sidebar became destinations only.
describe("Sign-out (AS-1)", () => {
  it("posts /auth/logout and clears the query cache on click", async () => {
    const user = userEvent.setup();
    let loggedOut = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        const method = input instanceof Request ? input.method : "GET";
        if (url.endsWith("/v1/auth/logout") && method === "POST") {
          loggedOut = true;
          return new Response(null, { status: 204 });
        }
        if (url.endsWith("/v1/me")) {
          return new Response(null, { status: loggedOut ? 401 : 200 });
        }
        return new Response(null, { status: 404 });
      }),
    );
    // Seed the ["me"] cache so we can observe sign-out resetting it: the gate's
    // re-probe hangs off this entry losing its data.
    const client = newClient();
    client.setQueryData(["me"], { user: { id: "u1", email: "ada@acme.test" } });
    window.location.hash = "#/deals";
    renderWith(client, <Shell onOpenSearch={ignoreSearch}>{null}</Shell>);
    expect(client.getQueryData(["me"])).toBeTruthy();
    // Sign-out lives inside the account menu, so it takes opening first.
    await user.click(screen.getByRole("button", { name: /Account$/ }));
    await user.click(screen.getByText("Sign out"));
    // POST fired and the ["me"] entry lost its data, so the auth gate
    // re-probes → 401 → login. This bites if useLogout's onSuccess stops
    // calling resetToSignedOut (screens/common.tsx).
    await waitFor(() => expect(loggedOut).toBe(true));
    await waitFor(() => expect(client.getQueryData(["me"])).toBeUndefined());
  });

  // queryClient.clear() alone empties the cache but does NOT force a mounted
  // ["me"] observer to refetch — a component still watching it can keep
  // rendering its last (stale, authenticated) snapshot. Render THROUGH the real
  // AuthGate (App, not just the rail in isolation) and prove sign-out actually
  // lands the user back on the login screen, driven by a real /v1/me re-probe —
  // not merely that the cache entry disappeared.
  it("drives the AuthGate back to the login screen after sign-out (bites on stale-cache regressions)", async () => {
    const user = userEvent.setup();
    let loggedOut = false;
    let meCalls = 0;
    vi.stubGlobal("localStorage", memoryStorage());
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        const method = input instanceof Request ? input.method : "GET";
        if (url.endsWith("/v1/auth/logout") && method === "POST") {
          loggedOut = true;
          return new Response(null, { status: 204 });
        }
        if (url.endsWith("/v1/me")) {
          meCalls += 1;
          if (loggedOut) {
            return new Response(JSON.stringify({ code: "unauthenticated" }), {
              status: 401,
              headers: { "Content-Type": "application/problem+json" },
            });
          }
          return new Response(
            JSON.stringify({ user: { id: "u1" }, roles: [], teams: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ code: "unavailable" }), {
          status: 503,
          headers: { "Content-Type": "application/problem+json" },
        });
      }),
    );
    renderWith(newClient(), <App />);

    // Authenticated: the chrome (and its account menu) is on screen. Boot may
    // probe more than once; what this test owns is the probe sign-out causes.
    const account = await screen.findByRole("button", { name: /Account$/ });
    expect(meCalls).toBeGreaterThanOrEqual(1);
    const probesBeforeSignOut = meCalls;

    await user.click(account);
    await user.click(screen.getByText("Sign out"));

    // The gate must re-probe /v1/me (not just drop the cache entry) and,
    // seeing 401, render the auth (signup/login) screen — the rail must be
    // gone. AuthScreen defaults to its signup mode, so assert on that
    // heading rather than assuming "Sign in" is the first thing shown.
    await screen.findByRole("heading", { name: "Sign in to Margince" });
    expect(screen.queryByRole("navigation")).toBeNull();
    expect(loggedOut).toBe(true);
    expect(meCalls).toBeGreaterThan(probesBeforeSignOut);
  });
});
