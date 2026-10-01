/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { type GrantSpec, meFixture } from "./app/mefixture";
import { createQueryClient } from "./app/queryclient";
import { parseHash, routeHash } from "./app/router";
import { LocaleProvider } from "./i18n";
import { memoryStorage, wizardRow } from "./testing/appharness";

// Two places decide, on their own, where a session belongs when the
// installation has not described itself, and each rewrites the hash to say so:
// AuthedApp's onboarding gate sends every route back to onboarding while
// GET /company answers 404, and the conversational shell's restore
// (screens/onboarding-conversation/restore.ts) decides whether the journey is
// finished and leaves. Those are the only two automatic navigators in the app;
// every other navigate() answers a click.
//
// When they disagree they rewrite the hash AT each other, and that is not a
// cosmetic redirect flicker. `useRoute` is a useSyncExternalStore over
// location.hash, so a navigate() inside a passive effect makes React's own
// store-instance check force a synchronous re-render, which runs the effect
// again — nested updates until React trips its limit with "Maximum update
// depth exceeded" and unmounts the shell. The app's error boundary sits above
// every route, so the whole product then reads "This view stopped working",
// including the address "Try again" returns to.
//
// The disagreement is reachable on a brand-new installation and nowhere else:
// the wizard row and the company profile are separate writes, and the connect
// act persists `step: "complete"` without requiring a saved profile. Both
// cases below are mounted through the REAL App, because a unit test of either
// half alone passes while the pair loops.

type InstallShape = Readonly<{
  /** GET /company; null answers 404 — the profile the gate looks for is absent. */
  companySaved: boolean;
  /**
   * GET /onboarding/state; false answers 404 — no wizard row has been written
   * at all, which is what a first deployment has before anyone touches it.
   */
  wizardStarted?: boolean;
}>;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The addresses the gate and the restore each send the reader to. A hash this
// app answers that neither of them chose is not part of the disagreement.
const GATE_TARGET = "#/onboarding/company";
const BRIEF = "#/home";

/**
 * An installation whose wizard row says `complete`, with or without the profile
 * that claim depends on.
 *
 * Everything past those two answers is the minimum the onboarding surface needs
 * to render: the assertion is about which ADDRESS the app settles on, and a
 * screen that threw for a missing fixture would fail these tests for an
 * unrelated reason.
 */
function completedWizardFetch(shape: InstallShape) {
  return async (input: Request | string | URL) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const path = new URL(url, "http://localhost").pathname.replace(/^\/v1/, "");
    const method = request?.method ?? "GET";
    if (path === "/me") {
      return json(meFixture({ roles: ["admin"], seat: "full" }));
    }
    if (path === "/company") {
      return shape.companySaved
        ? json({
            company_id: "018f3a1b-0000-7000-8000-0000000000a1",
            display_name: "Gradion",
            website: "gradion.com",
            offer_summary: "Revenue software for manufacturers",
            icp: "Mid-market manufacturers",
          })
        : json({ detail: "no company yet" }, 404);
    }
    if (path === "/onboarding/state" && method === "GET") {
      if (shape.wizardStarted === false) {
        return json({ detail: "no onboarding state yet" }, 404);
      }
      return json({
        path: "creator",
        step: "complete",
        source_mode: "website",
        website_url: "https://gradion.com",
        site_read_id: null,
        company_draft: {},
        selected_fact_keys: [],
        voice_skipped: false,
        connect_skipped: false,
        version: 4,
        completed_at: null,
        created_at: "2026-08-20T08:00:00Z",
        updated_at: "2026-08-20T09:00:00Z",
      });
    }
    if (path === "/company/context/capabilities") {
      return json({
        onboarding_enabled: true,
        read_enabled: true,
        rollout: "ga",
      });
    }
    if (path === "/ai/profile") {
      return json({
        name: "Margince",
        kind: "ai",
        state: "configured",
        inference_mode: "cloud",
        providers: ["gemini"],
        configured_models: [],
      });
    }
    if (path === "/voice-profiles" || path === "/connectors") {
      return json({ data: [], page: {} });
    }
    return json({ data: [], page: {} });
  };
}

/** Every address the app moved to, in order, so a ping-pong is legible as one. */
/**
 * Every address the app MOVES to, by whichever primitive it moves with.
 *
 * A `hashchange` listener alone is not enough and stopped seeing anything the
 * moment redirects began replacing the entry rather than assigning to the hash:
 * `history.replaceState` fires no `hashchange` where the specification is
 * followed, so a loop would have counted zero moves and passed. What this test
 * is about is how many times the app decides to go somewhere, so it counts the
 * decisions.
 */
function recordHashChanges(): string[] {
  const seen: string[] = [];
  const at = () => window.location.hash;
  // ONE MOVE IS RECORDED ONCE, whichever channel announces it.
  //
  // The two channels overlap, and by how much depends on the environment.
  // `hashchange` is specified to fire for an ASSIGNMENT to the hash and not for
  // a history write — jsdom follows that, happy-dom fires it for
  // pushState and replaceState too. Counting both channels blind therefore
  // records every redirect twice under one environment and once under the
  // other, and the count is the whole assertion: one move looks like a loop.
  //
  // So a history write claims the hashchange it causes. A SECOND move to the
  // same address arrives with nothing claimed and is still counted, which is
  // the recurrence these cases are looking for.
  let claimed: string | null = null;
  window.addEventListener("hashchange", () => {
    if (claimed === at()) {
      claimed = null;
      return;
    }
    seen.push(at());
  });
  for (const write of ["pushState", "replaceState"] as const) {
    // The prototype's own, not whatever is on `history` right now: binding the
    // latter binds the PREVIOUS test's spy, and the two then call each other
    // until the stack runs out.
    const original = History.prototype[write];
    vi.spyOn(window.history, write).mockImplementation((...args) => {
      original.apply(window.history, args);
      // A stamp on the entry the reader is already on names it; it is not a
      // move, and it passes no URL.
      if (args[2] !== undefined && args[2] !== null) {
        claimed = at();
        seen.push(at());
      }
    });
  }
  return seen;
}

function renderApp() {
  render(
    // StrictMode, because that is what main.tsx mounts: the double-invoked
    // effects are part of the shape a redirect loop is discovered in.
    <StrictMode>
      <QueryClientProvider client={createQueryClient()}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>
    </StrictMode>,
  );
}

function mount(shape: InstallShape, at: string): string[] {
  window.location.hash = at;
  vi.stubGlobal("fetch", vi.fn(completedWizardFetch(shape)));
  const moves = recordHashChanges();
  renderApp();
  return moves;
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
});

// Both cases count the app's moves rather than comparing them to a literal
// sequence: jsdom announces the starting hash the test itself set, which is the
// reader's arrival and not a move the app made. A loop shows up as a target
// RECURRING, which is asserted separately from the settled address — a loop
// that happens to stop on the right hash is still the defect.
describe("the onboarding gate and the wizard's restore", () => {
  it("agree on onboarding while the company profile is absent", async () => {
    const moves = mount({ companySaved: false }, BRIEF);

    // The company act reopens rather than reporting a completion the profile
    // does not support, so the gate's destination is where the reader stays.
    expect(await screen.findByLabelText(/Website address/)).toBeTruthy();
    await waitFor(() => {
      expect(window.location.hash).toBe(GATE_TARGET);
    });
    expect(moves.filter((hash) => hash === GATE_TARGET)).toHaveLength(1);
    expect(moves.at(-1)).toBe(GATE_TARGET);
  });

  it("agree on onboarding when no wizard row exists at all", async () => {
    // The state a real first deployment is in, and the one the other two cases
    // cannot reach: both stub a "complete" wizard row, so they exercise a
    // completion the profile does not support. Here nothing has been written —
    // no company, no wizard row — which is what an installation looks like
    // before anyone has touched it.
    //
    // It is asserted separately because the two sides agree here for a
    // different reason. Above, the restore REFUSES a completion it cannot back;
    // here there is no completion to refuse, and what has to hold is that an
    // absent row reads as "not started" rather than as an unknown the restore
    // resolves by leaving. A dev seed now writes a completed row, so this case
    // is no longer reachable from a seeded stack either way.
    const moves = mount({ companySaved: false, wizardStarted: false }, BRIEF);

    expect(await screen.findByLabelText(/Website address/)).toBeTruthy();
    await waitFor(() => {
      expect(window.location.hash).toBe(GATE_TARGET);
    });
    expect(moves.filter((hash) => hash === GATE_TARGET)).toHaveLength(1);
    expect(moves.at(-1)).toBe(GATE_TARGET);
  });

  it("agree on leaving once the profile backs the completion", async () => {
    // The mirror, and it is what keeps the guard above from being a way to
    // strand a finished installation on the onboarding screen: with the profile
    // saved, the completion is real and the restore leaves — once.
    const moves = mount({ companySaved: true }, GATE_TARGET);

    await waitFor(() => {
      expect(window.location.hash).toBe(BRIEF);
    });
    expect(moves.filter((hash) => hash === BRIEF)).toHaveLength(1);
    expect(moves.at(-1)).toBe(BRIEF);
  });

  it("leave no entry behind for Back to land on", async () => {
    // Both moves above are REDIRECTS: an address the product answers by sending
    // the reader somewhere else. Pushed, each leaves the address it came from
    // in history — so Back returns to it, it redirects again, and the reader
    // cannot get out with the one key that exists for getting out of things.
    // The settled hash cannot see this; the depth of the stack can.
    // Measured from the reader's ARRIVAL, not from before it: setting the
    // starting hash is itself an entry, and counting it would leave the
    // assertion satisfied by a gate that pushed.
    window.location.hash = BRIEF;
    const onArrival = window.history.length;
    mount({ companySaved: false }, BRIEF);

    expect(await screen.findByLabelText(/Website address/)).toBeTruthy();
    await waitFor(() => {
      expect(window.location.hash).toBe(GATE_TARGET);
    });
    expect(window.history.length).toBe(onArrival);
  });
});

// The onboarding gate: an installation that has not saved its own company has
// nothing for any other screen to show, so an admin is sent to the company form
// (GET /company 404s until then). Any other seat implies a described
// installation, and every full seat is walked through its own journey until
// that is recorded as finished. The gate lives in the shell, not on the login
// path, because a live session never passes through login.
describe("onboarding gate", () => {
  const mount = () => {
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
  };

  // Every call the shell makes resolves; only /company's status and the
  // journey's own row vary, so the gate is the single thing under test. The
  // journey defaults to finished, which is what lets a described installation
  // stay where it was asked to go.
  const stubCompany = (
    status: number,
    journey: { row: unknown; status: number } = {
      row: wizardRow("complete"),
      status: 200,
    },
    session: {
      seat?: "full" | "read";
      roles?: string[];
      allow?: GrantSpec;
      onboarding?: boolean;
    } = {},
  ) =>
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify(
              meFixture({
                roles: session.roles ?? ["admin"],
                seat: session.seat,
                allow: session.allow,
              }),
            ),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        // The rollout stage: the journey runs at `onboarding`, unless a case
        // sets the installation below it.
        if (url.endsWith("/v1/company/context/capabilities")) {
          return new Response(
            JSON.stringify({
              onboarding_enabled: session.onboarding ?? true,
              read_enabled: true,
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.endsWith("/v1/onboarding/state")) {
          return new Response(JSON.stringify(journey.row), {
            status: journey.status,
            headers: {
              "Content-Type":
                journey.status === 200
                  ? "application/json"
                  : "application/problem+json",
            },
          });
        }
        if (url.endsWith("/v1/company")) {
          return status === 200
            ? new Response(
                JSON.stringify({
                  company_id: "o1",
                  display_name: "Acme GmbH",
                }),
                {
                  status: 200,
                  headers: { "Content-Type": "application/json" },
                },
              )
            : new Response(JSON.stringify({ code: "not_found" }), {
                status,
                headers: { "Content-Type": "application/problem+json" },
              });
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

  it("sends an installation that has not described itself to the company form", async () => {
    stubCompany(404);
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  it("holds on every navigation — steering away mid-onboarding lands back on the form", async () => {
    stubCompany(404);
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );

    // The palette, a typed hash, a stray link: any client-side navigation
    // away from onboarding must be turned around, not just the first load.
    window.location.hash = "#/contacts";
    window.dispatchEvent(new HashChangeEvent("hashchange"));
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  // The gate's second half: a described installation still walks every human
  // whose own journey is unfinished through it. That is how a member invited
  // later trains their voice and connects their mailbox as the creator did.
  it("sends a human with no journey of their own to onboarding, company or not", async () => {
    window.location.hash = "#/contacts";
    stubCompany(200, { row: { code: "not_found" }, status: 404 });
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  it("sends a human whose journey stopped short back into it", async () => {
    window.location.hash = "#/contacts";
    stubCompany(200, { row: wizardRow("voice"), status: 200 });
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  // A read seat cannot write the checkpoint the journey ends on, so a gate
  // that held it would hold it forever.
  it("leaves a read seat alone, whatever its journey says", async () => {
    window.location.hash = "#/contacts";
    stubCompany(
      200,
      { row: { code: "not_found" }, status: 404 },
      { seat: "read" },
    );
    mount();
    await screen.findByRole("navigation", { name: "Primary navigation" });
    expect(routeHash(parseHash(window.location.hash))).toBe("#/contacts");
  });

  // GET /company answers only an admin, so no other seat asks for it. It needs
  // no answer either: nobody is invited before the company is described.
  const requestsEndingIn = (path: string) =>
    vi
      .mocked(fetch)
      .mock.calls.filter(([input]) =>
        String(input instanceof Request ? input.url : input).endsWith(path),
      );
  const companyReads = () => requestsEndingIn("/v1/company");
  const ROLLOUT = "/v1/company/context/capabilities";

  // Wraps the stubbed fetch so every request to `path` waits for the returned
  // release; the rest answer at once.
  const holdAnswersTo = (path: string) => {
    const served = vi.mocked(fetch);
    let release = () => {};
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        if (
          String(input instanceof Request ? input.url : input).endsWith(path)
        ) {
          await held;
        }
        return served(input);
      }),
    );
    return release;
  };

  it("leaves a non-admin with a finished journey on its route, unasked", async () => {
    window.location.hash = "#/contacts";
    stubCompany(403, undefined, { roles: ["rep"] });
    mount();
    await screen.findByRole("navigation", { name: "Primary navigation" });
    expect(routeHash(parseHash(window.location.hash))).toBe("#/contacts");
    expect(companyReads()).toHaveLength(0);
  });

  it.each([
    ["no journey row", { row: { code: "not_found" }, status: 404 }],
    ["an unfinished journey", { row: wizardRow("voice"), status: 200 }],
  ])(
    "walks a non-admin with %s through the member journey, unasked",
    async (_, journey) => {
      window.location.hash = "#/contacts";
      stubCompany(403, journey, { roles: ["rep"] });
      mount();
      expect(await screen.findByText(/Train your writing voice/)).toBeTruthy();
      expect(window.location.hash).toBe("#/onboarding/company");
      expect(companyReads()).toHaveLength(0);
    },
  );

  // ops holds automation:update, so its journey names the configured model:
  // the model read follows the grant, not the admin role.
  it("walks an ops seat with no row through the member journey, model named", async () => {
    window.location.hash = "#/contacts";
    stubCompany(
      403,
      { row: { code: "not_found" }, status: 404 },
      { roles: ["ops"], allow: { automation: ["update"] } },
    );
    mount();
    expect(await screen.findByText(/Train your writing voice/)).toBeTruthy();
    expect(window.location.hash).toBe("#/onboarding/company");
    expect(companyReads()).toHaveLength(0);
    await waitFor(() =>
      expect(requestsEndingIn("/v1/ai/profile")).toHaveLength(1),
    );
  });

  it("leaves a non-admin on a read seat alone, whatever its journey says", async () => {
    window.location.hash = "#/contacts";
    stubCompany(
      403,
      { row: { code: "not_found" }, status: 404 },
      { roles: ["rep"], seat: "read" },
    );
    mount();
    await screen.findByRole("navigation", { name: "Primary navigation" });
    expect(routeHash(parseHash(window.location.hash))).toBe("#/contacts");
    expect(companyReads()).toHaveLength(0);
  });

  // Below the `onboarding` rollout stage there is no journey, only the manual
  // company form, which a rep cannot save and an admin's save never finishes.
  it.each([
    ["a non-admin", 403, ["rep"]],
    ["an admin", 200, ["admin"]],
  ])(
    "leaves %s alone when the installation has no journey to walk",
    async (_, status, roles) => {
      window.location.hash = "#/contacts";
      stubCompany(
        status,
        { row: { code: "not_found" }, status: 404 },
        {
          roles,
          onboarding: false,
        },
      );
      mount();
      await screen.findByRole("navigation", { name: "Primary navigation" });
      expect(routeHash(parseHash(window.location.hash))).toBe("#/contacts");
    },
  );

  // The first half of the gate reads no rollout: the manual company form is
  // what an installation below the `onboarding` stage describes itself with.
  it("sends an undescribed installation to the company form below the onboarding stage", async () => {
    window.location.hash = "#/contacts";
    stubCompany(404, undefined, { onboarding: false });
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  // A shell painted before the rollout answers is a landing page the gate may
  // then pull away from under the reader, so the splash waits for it.
  it("holds the splash until the rollout says whether there is a journey", async () => {
    window.location.hash = "#/contacts";
    stubCompany(
      403,
      { row: { code: "not_found" }, status: 404 },
      {
        roles: ["rep"],
      },
    );
    const release = holdAnswersTo(ROLLOUT);
    mount();
    await waitFor(() => expect(requestsEndingIn(ROLLOUT)).not.toHaveLength(0));
    expect(
      screen.queryByRole("navigation", { name: "Primary navigation" }),
    ).toBeNull();
    release();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  // Serialised behind the company read, the rollout would add a round trip to
  // every admin's splash, so it goes out while the company is still unanswered.
  it("asks an admin's rollout beside the company read, not behind it", async () => {
    window.location.hash = "#/contacts";
    stubCompany(200, { row: { code: "not_found" }, status: 404 });
    const release = holdAnswersTo("/v1/company");
    mount();
    await waitFor(() => expect(requestsEndingIn(ROLLOUT)).not.toHaveLength(0));
    expect(
      screen.queryByRole("navigation", { name: "Primary navigation" }),
    ).toBeNull();
    release();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });

  it("leaves a described installation on the route it asked for", async () => {
    window.location.hash = "#/contacts";
    stubCompany(200);
    mount();
    // The company resolves before this settles, so a gate that redirected
    // would have replaced the hash by now.
    await screen.findByRole("navigation", { name: "Primary navigation" });
    // The SCREEN, not the whole address. A list spells its own opening dials
    // into the hash on arrival, so contacts settles at `#/contacts?sort=…` a
    // moment after the shell renders; an equality against the bare address
    // holds only while that write is still pending. Where the gate left the
    // reader is this test's claim — how the list is sorted is contacts.tsx's.
    expect(routeHash(parseHash(window.location.hash))).toBe("#/contacts");
  });

  // A pending /oauth/authorize request lives entirely in the hash (the
  // client_id/scope/consent-nonce query string) — navigate() rewrites
  // location.hash, so a gate redirect here would destroy the request with no
  // way to recover it, unlike an ordinary screen a human can simply re-visit.
  it("does not redirect away from oauth-consent when the company is undescribed", async () => {
    const pendingHash =
      "#/oauth-consent?client_id=c1&scope=read&consent=nonce123";
    window.location.hash = pendingHash;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.endsWith("/v1/me")) {
          return new Response(
            JSON.stringify({ user: { id: "u1" }, roles: ["admin"], teams: [] }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (url.endsWith("/v1/company")) {
          return new Response(JSON.stringify({ code: "not_found" }), {
            status: 404,
            headers: { "Content-Type": "application/problem+json" },
          });
        }
        if (url.includes("/oauth/consent-request")) {
          return new Response(
            JSON.stringify({
              client_name: "Acme Client",
              offline: false,
              scopes: ["read"],
            }),
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response(JSON.stringify({ data: [], page: {} }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    mount();
    // The consent screen itself is proof the gate never fired — an
    // onboarding redirect would have replaced the hash before this renders.
    expect(
      await screen.findByRole("heading", { name: "Authorize access" }),
    ).toBeTruthy();
    expect(window.location.hash).toBe(pendingHash);
  });

  // The control for the exemption above: it must be scoped to the consent
  // route, not a gate that stopped firing. This is the third premise the gate
  // has to answer — an ordinary route named in the hash on FIRST load (the
  // cases above cover an empty hash, and a hashchange after mount).
  it("still redirects an ordinary screen away when the company is undescribed", async () => {
    window.location.hash = "#/contacts";
    stubCompany(404);
    mount();
    await waitFor(() =>
      expect(window.location.hash).toBe("#/onboarding/company"),
    );
  });
});
