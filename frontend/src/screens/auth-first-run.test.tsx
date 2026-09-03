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
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { AuthScreen } from "./auth";

// Split out of auth.test.tsx (frontend/AGENTS.md: test files split at 1000
// lines, and that file was already over it) rather than sharing its helpers:
// same pattern as company-act-refusal.test.tsx beside company-act.test.tsx.
// This file pins ONE thing: the handover sentence a first-run installation
// (`capabilities.data.first_run === true`) says instead of the returning one,
// on the login view only, with every other sentence and the frame around
// them exactly as auth.test.tsx already pins them.

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
// (see the split note above). `first_run` is optional here so the first case
// can send the probe WITHOUT it: absence is a real wire state, and the
// presentation must decay to the ordinary sign-in on it.
function stubApi(
  capabilities: Omit<
    components["schemas"]["AuthCapabilities"],
    "oidc_providers" | "first_run"
  > & { first_run?: boolean },
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

const RETURNING = "First, let me make sure it’s really you…";
const FIRST_RUN = "Sign in and we’ll get started.";

describe("AuthScreen on a first-run installation", () => {
  it("says the returning handover when first_run is absent or false", async () => {
    stubApi({ password: true, password_reset: false }, () => ok(200));
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText(RETURNING)).toBeTruthy();
    expect(screen.queryByText(FIRST_RUN)).toBeNull();
    cleanup();

    stubApi({ password: true, password_reset: false, first_run: false }, () =>
      ok(200),
    );
    render(<AuthScreen onAuthed={vi.fn()} />);
    expect(await screen.findByText(RETURNING)).toBeTruthy();
    expect(screen.queryByText(FIRST_RUN)).toBeNull();
  });

  it("says a handover that claims no recognition, on the SAME frame", async () => {
    stubApi({ password: true, password_reset: false, first_run: true }, () =>
      ok(200, { user: {}, roles: [], teams: [] }),
    );
    const { container } = render(<AuthScreen onAuthed={vi.fn()} />);

    // `first_run` derives from the anonymous capabilities probe, which has
    // not resolved on the first render: the returning line is what renders
    // while it is in flight, by design (absent decays to false), so this waits
    // for the probe rather than reading the sentence synchronously.
    expect(await screen.findByText(FIRST_RUN)).toBeTruthy();
    expect(screen.queryByText(RETURNING)).toBeNull();
    // ADR-0076 Decision 1 still holds: the task is first in the DOM.
    const task = container.querySelector(".auth-task");
    const identity = container.querySelector(".auth-identity-col");
    expect(task).toBeTruthy();
    expect(identity).toBeTruthy();
    expect(precedes(task, identity)).toBe(true);
    // The task's own h1 is unchanged: no second, first-run-only heading.
    expect(await screen.findByText("Sign in to Margince")).toBeTruthy();
    // The identity region says what it always says (ADR-0076 Decision 2's
    // closed list); first run swaps one sentence, it does not add a voice.
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
    const user = userEvent.setup();

    // No "Begin" or similar gate between the greeting and the form: the
    // fields are on the page from the first render.
    await user.type(await screen.findByLabelText("Email"), "ada@example.com");
    await user.type(
      screen.getByLabelText("Password"),
      "correct-horse-battery{enter}",
    );

    await waitFor(() => expect(onAuthed).toHaveBeenCalled());
  });
});
