/** @vitest-environment happy-dom */
import {
  focusManager,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { api } from "./api/client";
import { pickOption } from "./design-system/select-testing";
import { LocaleProvider } from "./i18n";
import { sessionRefusalWatch } from "./lib/session";
import {
  memoryStorage,
  sessionOnlyFetch,
  wizardRow,
} from "./testing/appharness";
import { meFixture } from "./testing/mefixture";

// B-EP09.17: the locale switch flips the whole UI between DE and EN. With the
// browser asking for a language we don't ship, the app mounts in the A100
// fallback (en); one click renders the German chrome. The browser-level e2e
// twin of this test rides the 09.22 harness.
//
// The shell only renders behind a session: App probes GET /v1/me and shows the
// authenticated chrome once it is 200. The test seeds a workspace slug + a
// stubbed /me so the rail is reached (the signup/login gate has its own test).

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  // Pin the browser language to one we don't ship so mount resolves to the
  // A100 fallback deterministically, independent of the CI machine's locale.
  Object.defineProperty(globalThis.navigator, "languages", {
    value: ["fr-FR"],
    configurable: true,
  });
  // Only the session probe succeeds; the Brief's own data calls fail and
  // fall to their QueryGate error state (the rail still renders — that is what
  // this test asserts). Routing by URL keeps the stub honest per endpoint.
  vi.stubGlobal("fetch", vi.fn(sessionOnlyFetch()));
  // Every test here mounts at an address, so every test states the one it
  // mounts at. Inheriting whatever the last one left is a dependency on which
  // files a worker happened to reuse, and it holds only while the pool keeps
  // handing each file a fresh jsdom.
  window.location.hash = "";
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("the custom-fields admin, at its address inside settings", () => {
  // It used to be a route of its own, reached by a card whose only job was to
  // send you there. It is a section of Settings → Data model now, so the claim
  // this test makes is that the surface still MOUNTS through the real router —
  // and the address it mounts at is the one that changed.
  // The palette's half of #3850 ends here: it owns no page, so it puts the
  // message in the ADDRESS and the router hands it to the screen. The screen's
  // own test proves it opens what it is given; this proves the route gives it.
  it("opens the message a search address names", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(JSON.stringify(meFixture({ roles: ["admin"] })), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        // A finished journey, or the gate's second half takes the address.
        if (url.endsWith("/v1/onboarding/state")) {
          return new Response(JSON.stringify(wizardRow("complete")), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        // A saved company profile, or the onboarding gate takes the address
        // and the screen under test never mounts.
        if (url.endsWith("/v1/company")) {
          return new Response(
            JSON.stringify({
              company_id: "018f3a1b-0000-7000-8000-0000000000a1",
              display_name: "Gradion",
              website: "gradion.com",
              offer_summary: "Revenue software for manufacturers",
              icp: "Mid-market manufacturers",
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.includes("/email-presentation")) {
          return new Response(JSON.stringify({ code: "service_unavailable" }), {
            status: 503,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        return new Response(
          JSON.stringify({
            data: [],
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    window.location.hash = "#/search/renewal/a1";
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    // That it ASKS for this message is the claim. What the drawer then draws
    // is emaildetail's own contract, and the read is stubbed as failing here
    // precisely so this cannot accidentally assert on that.
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some((call) =>
            String(call[0] instanceof Request ? call[0].url : call[0]).includes(
              "/activities/a1/email-presentation",
            ),
          ),
      ).toBe(true),
    );
  });

  it("mounts the field builder on the Data model page", async () => {
    // Routing is under test; wait for the real lazy module before timing UI queries.
    await import("./features/settings/settings");
    // Every query the surface fires must resolve, or QueryGate paints its error
    // card instead of the heading: /me (an admin holding the custom_field write
    // the entry is gated on), the per-object field list, and the audit rail.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify(
              meFixture({
                roles: ["admin"],
                allow: { custom_field: ["read", "create", "update"] },
              }),
            ),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.includes("/v1/custom-fields")) {
          return new Response(JSON.stringify({ data: [], page: {} }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.includes("/v1/audit-log")) {
          return new Response(JSON.stringify({ data: [] }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(JSON.stringify({ code: "service_unavailable" }), {
          status: 503,
          headers: { "Content-Type": "application/problem+json" },
        });
      }),
    );
    // The custom-field editor's own page since the data-model entry split into
    // five. `data-model` still resolves — it is a declared alias — but naming
    // the current page means this test fails if the page moves rather than
    // silently riding a redirect.
    window.location.hash = "#/settings/fields";
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    // The SURFACE's own section header, at level 2. The shell's page head titles
    // the PAGE — "Data model" — so anchoring at level 1 would pass even if this
    // section never mounted.
    // WAITING FOR A CHUNK, not for a request. The settings screen arrives on a
    // chunk of its own and the shell no longer carries it — the catalog, the
    // addresses and the visibility predicate moved out so that src/app/** could
    // ask its three questions without pulling every settings card into the
    // always-loaded bundle. What the shell stopped paying for on every page, a
    // settings route now pays once on arrival, and in jsdom that import is
    // resolved by the test runner rather than served from a browser cache.
    //
    // It lands just past the 1000ms default: this passes at 1500ms on an idle
    // machine and failed at the default on a loaded runner, which is the wrong
    // way round for a threshold to sit. Stated as a literal because
    // scripts/test-budget.test.ts folds these numbers to check them against the
    // per-test ceiling, and a timeout it cannot read is a budget nobody checked.
    expect(
      await screen.findByRole(
        "heading",
        { level: 2, name: "Custom fields" },
        { timeout: 3000 },
      ),
    ).toBeTruthy();
  });
});

// The VANILLA extension lane, end to end through the real router and the real
// registry. This is the lane a fresh clone, a core developer's `pnpm dev` and
// the web image all build: the committed empty-tree stub, where every unit
// name misses. What must hold is that #/ext/<anything> still renders the
// authenticated shell with an honest not-found card — not a blank frame, not a
// crash, and not the "not built yet" copy, which would be a different (and
// false) claim about the same URL.
//
// The composed half of the pair is lib/extensions.test.ts, which hands the
// generator's own descriptor shape to the same lookup. Rendering a composed
// unit here is not possible without build/composition/ existing, and this
// suite must pass in a tree where it does not.
describe("extension routes (vanilla registry)", () => {
  it("renders the honest not-found card for a unit no installation composed", async () => {
    window.location.hash = "#/ext/notes";
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText(
        "No extension named \u201Cnotes\u201D is enabled on this installation.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText("This screen is not available yet.")).toBeNull();
  });
});

describe("locale switch", () => {
  it("mounts in English (A100) and flips the chrome to German on switch", async () => {
    await import("./features/settings/settings");
    // The seat keeps the language before the page switches, so the write must
    // be answered; every other read stays the session-only stub.
    const sessionOnly = sessionOnlyFetch();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const request = input instanceof Request ? input : undefined;
        if (
          request?.method === "PUT" &&
          request.url.endsWith("/v1/me/locale")
        ) {
          return new Response(JSON.stringify({ locale: "de" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return sessionOnly(input);
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={client}>
        <LocaleProvider>
          <App />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    // English default: once the session resolves, the rail carries English labels
    expect(await screen.findByRole("link", { name: "Contacts" })).toBeTruthy();
    // The language is a preference of this contact rather than a destination, so
    // it lives on Settings → Account and reaching it is a navigation. Which is
    // also what makes this an app-level claim: the choice is made on one route
    // and has to hold on the next one, not just inside the card that made it.
    window.location.hash = "#/settings/account";
    // The settings chunk again, for the reason spelled out on the Data model
    // case above: this waits for the module to arrive, not for a request.
    await pickOption(
      userEvent.setup(),
      await screen.findByRole(
        "combobox",
        { name: "Language" },
        { timeout: 3000 },
      ),
      "Deutsch",
    );
    // The language is drafted until the page's save bar writes it.
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Save" }));
    await screen.findByRole("combobox", { name: "Sprache" });

    window.location.hash = "#/home";
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Kontakte" })).toBeTruthy(),
    );
    expect(screen.queryByRole("link", { name: "Contacts" })).toBeNull();
  });
});

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <App />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("auth boundary states (login spec §4)", () => {
  const probe = (status: number, code: string) =>
    vi.fn(async (input: Request | string | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.endsWith("/v1/me")) {
        return new Response(JSON.stringify({ code }), {
          status,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      return new Response(JSON.stringify({}), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });

  it("renders login on 401 — not signed in is an authentication state", async () => {
    vi.stubGlobal("fetch", probe(401, "unauthorized"));
    mount();
    expect(
      await screen.findByRole("heading", { name: "Sign in to Margince" }),
    ).toBeTruthy();
  });

  it("renders the connection problem on 5xx — an outage is never a login", async () => {
    vi.stubGlobal("fetch", probe(500, "internal"));
    mount();
    expect(
      await screen.findByText("Margince could not be reached"),
    ).toBeTruthy();
    expect(screen.queryByLabelText("Email")).toBeNull();
  });

  it("renders installation-unavailable on 503 and retry re-probes /me", async () => {
    const fetchMock = probe(503, "service_unavailable");
    vi.stubGlobal("fetch", fetchMock);
    mount();
    expect(await screen.findByText("Installation not ready")).toBeTruthy();
    const before = fetchMock.mock.calls.length;
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(fetchMock.mock.calls.length).toBeGreaterThan(before),
    );
  });

  // ADR-0105: "not ready" is two product states, and only one of them has
  // something the contact in front of the browser can do.
  it("offers the claim screen when the unready installation is waiting to be claimed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(JSON.stringify({ code: "service_unavailable" }), {
            status: 503,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        if (url.endsWith("/setup/status")) {
          return new Response(JSON.stringify({ claimable: true }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(
      await screen.findByRole("heading", { name: "Claim this installation" }),
    ).toBeTruthy();
    // The availability message is REPLACED, not stacked beside it.
    expect(screen.queryByText("Installation not ready")).toBeNull();
  });

  it("keeps the availability message when the setup probe cannot answer", async () => {
    // A probe that fails must not replace a true message with a claim screen
    // the installation may not actually offer.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/setup/status")) throw new TypeError("probe down");
        if (url.endsWith("/v1/me")) {
          return new Response(JSON.stringify({ code: "service_unavailable" }), {
            status: 503,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(await screen.findByText("Installation not ready")).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Claim this installation" }),
    ).toBeNull();
  });

  it("recovers the claim screen when retry follows a failed setup probe", async () => {
    // The probe resolves rather than throws, so a one-off failure caches a
    // `false`. Without refetching it on retry, an installation that has been
    // claimable all along would stay behind the availability screen until the
    // page was reloaded.
    let probeCalls = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/setup/status")) {
          probeCalls += 1;
          if (probeCalls === 1) throw new TypeError("probe down");
          return new Response(JSON.stringify({ claimable: true }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.endsWith("/v1/me")) {
          return new Response(JSON.stringify({ code: "service_unavailable" }), {
            status: 503,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(await screen.findByText("Installation not ready")).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(
      await screen.findByRole("heading", { name: "Claim this installation" }),
    ).toBeTruthy();
  });

  // The account is authenticated and its credentials are correct, so
  // the login screen would loop — using them again lands in the same refusal.
  it("offers the password change when the account still holds an operator's password", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify({ code: "password_change_required", detail: "x" }),
            {
              status: 403,
              headers: { "Content-Type": "application/problem+json" },
            },
          );
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(
      await screen.findByRole("heading", { name: "Set your own password" }),
    ).toBeTruthy();
    // Not the login screen: the password they have is correct, and being asked
    // for it again explains nothing.
    expect(
      screen.queryByRole("heading", { name: "Sign in to Margince" }),
    ).toBeNull();
  });

  it("offers two-step setup when the installation requires a factor the account lacks", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify({ code: "mfa_enrolment_required", detail: "x" }),
            {
              status: 403,
              headers: { "Content-Type": "application/problem+json" },
            },
          );
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(
      await screen.findByRole("heading", { name: "Set up two-step sign-in" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Sign in to Margince" }),
    ).toBeNull();
  });

  it("lands a sign-in that the factor requirement confines on the setup, not on an error", async () => {
    let signedIn = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/auth/login")) {
          signedIn = true;
          return new Response(JSON.stringify({}), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify(
              signedIn
                ? { code: "mfa_enrolment_required" }
                : { code: "unauthorized" },
            ),
            {
              status: signedIn ? 403 : 401,
              headers: { "Content-Type": "application/problem+json" },
            },
          );
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    await userEvent.type(await screen.findByLabelText("Email"), "a@b.com");
    await userEvent.type(
      screen.getByLabelText("Password"),
      "a password{enter}",
    );
    expect(
      await screen.findByRole("heading", { name: "Set up two-step sign-in" }),
    ).toBeTruthy();
    expect(screen.queryByText(/could not be reached/i)).toBeNull();
  });

  it("still renders login for an ordinary 403 that is not a forced change", async () => {
    // The kind is decided by the machine code, not the status: only the
    // refusal that names itself gets the change screen.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(JSON.stringify({ code: "permission_denied" }), {
            status: 403,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        return new Response(JSON.stringify({}), { status: 200 });
      }),
    );
    mount();
    expect(
      await screen.findByRole("heading", { name: "Sign in to Margince" }),
    ).toBeTruthy();
  });

  it("renders the connection problem when the probe cannot reach the API at all", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("network down");
      }),
    );
    mount();
    expect(
      await screen.findByText("Margince could not be reached"),
    ).toBeTruthy();
  });
});

// The emailed reset link must reach the reset form regardless of whatever
// session the browser already carries — a stale/live cookie must never turn
// a password-reset link into the authenticated shell's unrecognised-route
// fallback, and a token redeemed to completion must not strand the
// following sign-in off the app's normal post-login redirect.
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

  it("reaches the Brief on the sign-in that follows a completed reset", async () => {
    // No session at the start: the ordinary case for a password reset. Once
    // the login below succeeds, /v1/me flips to authenticated — proving the
    // sign-in actually completed the redirect rather than getting stuck
    // because the reset route was still sitting in the hash.
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
    // The rail is proof the app reached the Brief, not proof merely that /v1/me
    // now resolves — a stale reset hash would instead leave login re-rendered
    // with nowhere for the post-login redirect to go.
    expect(
      await screen.findByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
  });

  it("reaches the Brief on a sign-in from a bare reset link that never carried a token", async () => {
    // No query string at all — a stale or hand-typed "#/reset-password" with
    // nothing to reset. No token reaches the screen, so this mounts
    // straight into the ordinary login form (never ResetForm, and never the
    // "Back to sign in" step that would otherwise have cleared the hash) — the
    // hash stays exactly "#/reset-password" through the whole sign-in.
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
    // The rail is proof the sign-in actually landed the reader on the app —
    // the non-empty "#/reset-password" hash LoginForm's own redirect check
    // preserves would otherwise leave this render stuck on the login form.
    expect(
      await screen.findByRole("navigation", { name: "Primary navigation" }),
    ).toBeTruthy();
  });
});

describe("a session confined while it is open", () => {
  it("swaps the open account page for the setup once /me answers the factor requirement", async () => {
    let confined = false;
    let meReads = 0;
    const refusal = () =>
      new Response(JSON.stringify({ code: "mfa_enrolment_required" }), {
        status: 403,
        headers: { "Content-Type": "application/problem+json" },
      });
    const ok = (body: unknown) =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const path = new URL(
          String(input instanceof Request ? input.url : input),
          "http://localhost",
        ).pathname;
        if (path === "/v1/me") {
          meReads += 1;
          return confined ? refusal() : ok(meFixture());
        }
        if (path === "/v1/me/mfa") {
          return ok({
            enrolled: false,
            confirmed: false,
            recovery_codes_left: 0,
          });
        }
        if (confined) return refusal();
        if (path === "/v1/company") {
          return ok({ company_id: "o1", display_name: "Acme" });
        }
        return ok({ data: [], page: {} });
      }),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const watch = sessionRefusalWatch(client);
    api.use(watch);
    window.location.hash = "#/settings/account";
    try {
      render(
        <QueryClientProvider client={client}>
          <LocaleProvider initial="en">
            <App />
          </LocaleProvider>
        </QueryClientProvider>,
      );
      await screen.findByText("Two-step sign-in");
      const readsBefore = meReads;

      confined = true;
      await client.invalidateQueries({ queryKey: ["me"] });

      expect(
        await screen.findByRole("heading", { name: "Set up two-step sign-in" }),
      ).toBeTruthy();
      expect(meReads - readsBefore).toBe(1);
    } finally {
      api.eject(watch);
    }
  });
});

describe("the forced two-step setup, held at the boundary", () => {
  const SECRET = "JBSWY3DPEHPK3PXP";
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/problem+json" },
    });

  // /me refuses until the factor is confirmed, then answers as the server
  // does: the refusal is lifted for the same session at once.
  function serveEnrolment({ lapsed = false } = {}) {
    let confirmed = false;
    let signedOut = false;
    let meReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const request = input instanceof Request ? input : null;
        const path = new URL(String(request?.url ?? input), "http://localhost")
          .pathname;
        if (path === "/v1/me") {
          meReads += 1;
          if (signedOut) return json({ code: "unauthorized" }, 401);
          return confirmed
            ? json(meFixture())
            : json({ code: "mfa_enrolment_required" }, 403);
        }
        if (path === "/v1/me/mfa/totp") {
          return json({
            secret: SECRET,
            otpauth_uri: `otpauth://totp/Margince:a%40b.test?secret=${SECRET}`,
          });
        }
        if (path === "/v1/me/mfa/totp/confirm") {
          if (lapsed) {
            signedOut = true;
            return json({ code: "unauthorized" }, 401);
          }
          confirmed = true;
          return json({ recovery_codes: ["aB3-dEf_ghIJkl"] });
        }
        if (path === "/v1/auth/logout") {
          signedOut = true;
          return new Response(null, { status: 204 });
        }
        if (path === "/v1/company") {
          return json({ company_id: "o1", display_name: "Acme" });
        }
        return json({ data: [], page: {} });
      }),
    );
    return { meReads: () => meReads };
  }

  it("keeps the recovery codes on screen through a focus refetch, until Done", async () => {
    const user = userEvent.setup();
    const server = serveEnrolment();
    window.location.hash = "#/settings/account";
    mount();

    await user.click(await screen.findByRole("button", { name: "Set up" }));
    await user.type(
      await screen.findByRole("textbox", { name: /code from the app/i }),
      "123 456{Enter}",
    );
    await screen.findByRole("textbox", { name: "Recovery codes" });

    const readsBefore = server.meReads();
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    await waitFor(() => expect(server.meReads()).toBeGreaterThan(readsBefore));
    focusManager.setFocused(undefined);

    expect(
      screen.getByRole("textbox", { name: "Recovery codes" }),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("heading", { name: "Set up two-step sign-in" }),
      ).toBeNull(),
    );
  });

  it("lets go of the setup when the account signs out instead", async () => {
    const user = userEvent.setup();
    serveEnrolment();
    mount();

    await user.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(
      await screen.findByRole("heading", { name: "Sign in to Margince" }),
    ).toBeTruthy();
  });

  it("sends a session that lapsed mid-setup to sign-in, not to a wrong code", async () => {
    const user = userEvent.setup();
    serveEnrolment({ lapsed: true });
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
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
      await user.click(await screen.findByRole("button", { name: "Set up" }));
      await user.type(
        await screen.findByRole("textbox", { name: /code from the app/i }),
        "123 456{Enter}",
      );
      expect(
        await screen.findByRole("heading", { name: "Sign in to Margince" }),
      ).toBeTruthy();
    } finally {
      api.eject(watch);
    }
  });
});
