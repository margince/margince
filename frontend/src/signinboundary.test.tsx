/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { api } from "./api/client";
import { createQueryClient } from "./app/queryclient";
import { LocaleProvider } from "./i18n";
import { sessionRefusalWatch } from "./lib/session";
import { memoryStorage } from "./testing/appharness";

beforeEach(() => {
  window.location.hash = "";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// A refused password leaves the sign-in surface standing.
//
// The probe holds no data while the reader is signed out, and a query with
// no data returns to `pending` for every refetch. The refusal watch re-asks
// /me after any non-/me 401, and a wrong password is one.
//
// So the boundary went from "signed out" to "nothing known yet" and back on
// each attempt. That took the form down and built a new one, losing the
// refusal and the email with it.
//
// Built on createQueryClient, the client the application runs. A hand-made
// one with other defaults does not re-ask, and the case then passes over
// the defect.
describe("a password the server refuses", () => {
  const problem = (body: unknown, status: number) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/problem+json" },
    });

  it("keeps the sign-in surface, its message and the email", async () => {
    vi.stubGlobal("localStorage", memoryStorage());
    // The re-ask is HELD until this case releases it, which is the window the
    // defect lives in: a clock would decide the outcome instead.
    let releaseProbe = () => {};
    const probeHeld = new Promise<void>((settle) => {
      releaseProbe = settle;
    });
    let probes = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const path = new URL(
          String(input instanceof Request ? input.url : input),
          "http://localhost",
        ).pathname;
        if (path.endsWith("/auth/capabilities")) {
          return problem(
            { password: true, password_reset: false, oidc_providers: [] },
            200,
          );
        }
        if (path.endsWith("/auth/login")) {
          return problem(
            { title: "unauthorized", detail: "invalid email or password" },
            401,
          );
        }
        probes += 1;
        if (probes > 1) {
          await probeHeld;
        }
        return problem({ title: "unauthorized", detail: "no session" }, 401);
      }),
    );
    const client = createQueryClient();
    const watch = sessionRefusalWatch(client);
    api.use(watch);
    try {
      render(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">
            <App />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      const email = await screen.findByLabelText("Email");
      await userEvent.type(email, "ada@example.com");
      await userEvent.type(screen.getByLabelText("Password"), "wrong{enter}");

      const alert = await screen.findByRole("alert");
      expect(alert.textContent).toContain("Sign-in failed");
      // The same input node: a remount would answer a different element.
      expect(screen.getByLabelText("Email")).toBe(email);
      expect(email).toHaveProperty("value", "ada@example.com");
      // The re-ask was still in flight for every assertion above, which is
      // what this case is about. Let it finish so nothing outlives the test.
      expect(probes).toBe(2);
      releaseProbe();
    } finally {
      api.eject(watch);
    }
  });
});
