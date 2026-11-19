/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  memoryStorage,
  renderApp as mount,
  sessionOnlyFetch,
} from "./testing/appharness";
// The screens these cases open, loaded while the file is collected. A screen's
// first import costs seconds; inside a case it races the case's own waiters.
import "./features/brief/brief";
import "./App.settings";
import "./features/settings/settings";
import "./features/sign-in/forcedmfa";
import "./features/voice-knowledge/search";

// The reset link reaches its form ahead of the session gate; the shell's
// own boundary states are in App.test.tsx.

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  // Pin the browser language to one we don't ship so mount resolves to the
  // A100 fallback deterministically, independent of the CI machine's locale.
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["fr-FR"],
    configurable: true,
  });
  // Only the session probe succeeds; a case stubs the rest it needs.
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  // Each case states the address it mounts at.
  window.location.hash = "";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("password-reset deep link", () => {
  const supportingRoutes = async (input: Request | string | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/auth/capabilities")) {
      return new Response(
        JSON.stringify({ oidc_providers: [], password: true }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    if (url.endsWith("/v1/assistant/profile")) {
      return new Response(
        JSON.stringify({
          name: "Margince",
          kind: "ai",
          state: "unconfigured",
          inference_mode: "none",
          providers: [],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }
    return null;
  };

  it("opens the reset form for an already-signed-in browser, not the pending screen", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const supporting = await supportingRoutes(input);
        if (supporting) return supporting;
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify({ user: { id: "u1" }, roles: [], teams: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    window.location.hash = "#/reset-password?token=live-session-token";
    mount();
    expect(await screen.findByLabelText("New password")).toBeTruthy();
  });

  it("keeps a signed-in browser in the app on a bare reset link", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const supporting = await supportingRoutes(input);
        if (supporting) return supporting;
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify({ user: { id: "u1" }, roles: [], teams: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.endsWith("/v1/company")) {
          return new Response(
            JSON.stringify({ company_id: "o1", display_name: "Acme" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    window.location.hash = "#/reset-password";
    mount();

    expect(
      await screen.findByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(window.location.hash).not.toContain("reset-password"),
    );
    expect(screen.queryByLabelText("Email")).toBeNull();
  });

  it("reaches the Brief on the sign-in that follows a completed reset", async () => {
    // No session at the start, the ordinary case for a reset. Once the login
    // below succeeds, /v1/me answers. The sign-in must then leave the reset
    // route rather than stay on it.
    let authenticated = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const supporting = await supportingRoutes(input);
        if (supporting) return supporting;
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return authenticated
            ? new Response(
                JSON.stringify({ user: { id: "u1" }, roles: [], teams: [] }),
                {
                  status: 200,
                  headers: { "Content-Type": "application/json" },
                },
              )
            : new Response(JSON.stringify({ code: "unauthorized" }), {
                status: 401,
                headers: { "Content-Type": "application/problem+json" },
              });
        }
        if (url.endsWith("/v1/auth/reset-password")) {
          return new Response(null, { status: 204 });
        }
        if (url.endsWith("/v1/auth/login")) {
          authenticated = true;
          return new Response(JSON.stringify({}), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.endsWith("/v1/company")) {
          return new Response(
            JSON.stringify({ company_id: "o1", display_name: "Acme" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    window.location.hash = "#/reset-password?token=completed-reset-token";
    mount();

    await userEvent.type(
      await screen.findByLabelText("New password"),
      "an entirely new password{enter}",
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Back to sign in" }),
    );
    await waitFor(() =>
      expect(window.location.hash).not.toContain("reset-password"),
    );
    await userEvent.type(await screen.findByLabelText("Email"), "a@b.com");
    await userEvent.type(
      screen.getByLabelText("Password"),
      "an entirely new password{enter}",
    );
    // The rail proves the app reached the Brief. A stale reset hash would
    // leave the login form on screen instead.
    expect(
      await screen.findByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
  });

  it("reaches the Brief on a sign-in from a bare reset link that never carried a token", async () => {
    // A stale or typed "#/reset-password" with no token has nothing to reset.
    // It mounts the ordinary login form, never ResetForm. The hash stays
    // "#/reset-password" through the whole sign-in.
    let authenticated = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const supporting = await supportingRoutes(input);
        if (supporting) return supporting;
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return authenticated
            ? new Response(
                JSON.stringify({ user: { id: "u1" }, roles: [], teams: [] }),
                {
                  status: 200,
                  headers: { "Content-Type": "application/json" },
                },
              )
            : new Response(JSON.stringify({ code: "unauthorized" }), {
                status: 401,
                headers: { "Content-Type": "application/problem+json" },
              });
        }
        if (url.endsWith("/v1/auth/login")) {
          authenticated = true;
          return new Response(JSON.stringify({}), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.endsWith("/v1/company")) {
          return new Response(
            JSON.stringify({ company_id: "o1", display_name: "Acme" }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    window.location.hash = "#/reset-password";
    mount();

    await userEvent.type(await screen.findByLabelText("Email"), "a@b.com");
    await userEvent.type(
      screen.getByLabelText("Password"),
      "an entirely new password{enter}",
    );
    // The rail proves the sign-in reached the app. LoginForm keeps a non-empty
    // hash, so "#/reset-password" could otherwise hold the login form.
    expect(
      await screen.findByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
  });
});
