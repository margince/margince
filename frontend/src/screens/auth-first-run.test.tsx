/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { AuthScreen } from "./auth";

// Split out of auth.test.tsx (frontend/AGENTS.md: test files split at 1000
// lines, and that file was already over it) rather than sharing its helpers:
// same pattern as company-act-refusal.test.tsx beside company-act.test.tsx.
// This file pins ONE thing: `AuthExperience`'s `welcome` presentation
// (auth-core.tsx), which renders only for the login view of a first-run
// installation (`capabilities.data.first_run === true`) and otherwise leaves
// every view of every installation exactly as auth.test.tsx already pins it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

// Same shape as auth.test.tsx's own stubApi, duplicated rather than imported
// (see the split note above) and carrying only what this file's cases need:
// `first_run`, which is not in schema.d.ts yet (see the same field read in
// auth.tsx) and is typed here on the test's own local shape, which is what
// lets this stub send the wire shape the backend is adding without a cast.
function stubApi(
  capabilities: {
    password: boolean;
    password_reset: boolean;
    first_run?: boolean;
  },
  respond: (request: Request) => Response | Promise<Response>,
  profile: Response = ok(200, {
    name: "Margince",
    kind: "ai",
    state: "unconfigured",
    inference_mode: "none",
    providers: [],
  }),
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: Request | string | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      if (new URL(request.url).pathname.endsWith("/auth/capabilities")) {
        return new Response(
          JSON.stringify({ oidc_providers: [], ...capabilities }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (new URL(request.url).pathname.endsWith("/assistant/profile")) {
        return profile;
      }
      return respond(request);
    }),
  );
}

const ok = (status: number, body?: unknown) =>
  new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

// Null-safe wrapper over `compareDocumentPosition`, so a DOM-order assertion
// can stay one `expect` instead of a non-null assertion on two querySelector
// results: a missing element fails the ordinary `expect(...).toBeTruthy()`
// beside it rather than throwing here.
function precedes(a: Element | null, b: Element | null): boolean {
  if (!a || !b) {
    return false;
  }
  return (
    (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0
  );
}

describe("AuthScreen first-run welcome", () => {
  it("renders the ordinary presentation when first_run is absent or false", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    const { container } = render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText("Sign in to Margince")).toBeTruthy();
    expect(
      container.querySelector<HTMLElement>(".auth-surface")?.dataset
        .authWelcome,
    ).toBe("false");
    cleanup();

    stubApi({ password: true, password_reset: false, first_run: false }, () =>
      ok(200),
    );
    const second = render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText("Sign in to Margince")).toBeTruthy();
    expect(
      second.container.querySelector<HTMLElement>(".auth-surface")?.dataset
        .authWelcome,
    ).toBe("false");
  });

  it("presents the SAME frame (task first in the DOM, the identity copy still the closed list of sentences) with the welcome attribute set", async () => {
    stubApi({ password: true, password_reset: false, first_run: true }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    const { container } = render(<AuthScreen onAuthed={vi.fn()} />);

    // `welcome` derives from the anonymous capabilities probe, which has not
    // resolved on the first render: the ordinary presentation is what
    // renders while it is in flight, by design (absent decays to false), so
    // this waits for the probe rather than reading the attribute
    // synchronously.
    const surface = container.querySelector<HTMLElement>(".auth-surface");
    await waitFor(() => expect(surface?.dataset.authWelcome).toBe("true"));
    // ADR-0076 Decision 1 still holds: the task is first in the DOM whichever
    // presentation renders.
    const task = container.querySelector(".auth-task");
    const identity = container.querySelector(".auth-identity-col");
    expect(task).toBeTruthy();
    expect(identity).toBeTruthy();
    // `DOCUMENT_POSITION_FOLLOWING` on the identity column means the task
    // node precedes it.
    expect(precedes(task, identity)).toBe(true);
    // The task's own h1 is unchanged: no second, welcome-only heading.
    expect(await screen.findByText("Sign in to Margince")).toBeTruthy();
    // The identity region says what it always says (ADR-0076 Decision 2's
    // closed list); the welcome sets it larger, it does not replace it.
    expect(
      screen.getByText("Hi, I’m Margince.", { selector: ".sr-only" }),
    ).toBeTruthy();
    // Same WCAG-visible labels the ordinary surface uses; not a placeholder.
    expect((await screen.findByLabelText("Email")).tagName).toBe("INPUT");
    expect(screen.getByLabelText("Password").tagName).toBe("INPUT");
    // §6.7 still holds here: this is an unauthenticated outcome too.
    expect(
      screen.getByText("Access to this organization is restricted."),
    ).toBeTruthy();
  });

  it("keeps the sign-in fields reachable with no extra click, and submits through the same behaviour", async () => {
    const onAuthed = vi.fn();
    stubApi({ password: true, password_reset: false, first_run: true }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    render(<AuthScreen onAuthed={onAuthed} />);

    // No "Begin" or similar gate between the welcome and the form: the fields
    // are on the page from the first render.
    await userEvent.type(
      await screen.findByLabelText("Email"),
      "ada@example.com",
    );
    await userEvent.type(
      screen.getByLabelText("Password"),
      "correct-horse-battery{enter}",
    );

    await waitFor(() => expect(onAuthed).toHaveBeenCalled());
  });
});
