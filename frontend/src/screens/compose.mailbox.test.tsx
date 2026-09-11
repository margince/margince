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
import { ComposeModal } from "./compose";

// Whose conversation the composer is answering.
//
// Its own file rather than another block in compose.test.tsx, which is already
// far past the size ceiling: a suite nobody can read is a suite nobody edits
// correctly.

type Activity = components["schemas"]["Activity"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function emptyResponse(status: number) {
  return new Response(null, { status });
}

// The client is returned so a test can re-render against the SAME cache — a
// fresh one would empty it, which is the opposite of the stale-entry case.
function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
    queryClient: client,
  };
}

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      const override = overrides[key];
      if (override) return override();
      return jsonResponse({});
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("whose mailbox the composer is answering from", () => {
  const colleaguesMail: Activity = {
    id: "act-charlotte",
    kind: "email",
    subject: "ACTION REQUIRED: IMMEDIATE PAYMENT REQUIRED",
    occurred_at: "2026-08-02T09:00:00Z",
    is_done: false,
    source: "gmail",
    captured_by: "connector:gmail:u-charlotte",
    created_at: "2026-08-02T09:00:00Z",
    updated_at: "2026-08-02T09:00:00Z",
  };

  // The reader is u-lars throughout; Charlotte is the seat whose mailbox took
  // delivery. Both are needed before the notice can decide anything, which is
  // why every case below stubs them.
  function mailboxRoutes(mailboxUserIds: string[]) {
    return {
      "GET /me": () =>
        jsonResponse({
          user: { id: "u-lars", display_name: "Lars", email: "l@demo.test" },
          role_keys: [],
        }),
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u-charlotte",
              display_name: "Charlotte Weber",
              email: "c@demo.test",
            },
            { id: "u-lars", display_name: "Lars", email: "l@demo.test" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /activities": () =>
        jsonResponse({
          data: [colleaguesMail],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /activities/act-charlotte": () => jsonResponse(colleaguesMail),
      "GET /activities/act-charlotte/reply-recipient": () =>
        jsonResponse({
          full_name: "Accounts Singapore",
          first_name: "Accounts",
          address: "accounts@vendor.test",
          mailbox_user_ids: mailboxUserIds,
        }),
    };
  }

  async function openOnColleaguesThread(mailboxUserIds: string[]) {
    stubRoutes(mailboxRoutes(mailboxUserIds));
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: /ACTION REQUIRED/,
      }),
    );
  }

  // The positive control. Every withholding case below is only meaningful
  // because this one proves the notice renders at all when it should.
  it("names the colleague whose mailbox took delivery", async () => {
    await openOnColleaguesThread(["u-charlotte"]);
    expect(await screen.findByText(/Charlotte Weber's mailbox/)).toBeTruthy();
    // And says where the reply comes from, which is the half that stops a
    // reader assuming the send goes out as Charlotte.
    expect(
      screen.getByText(/from your own mailbox, under your name/),
    ).toBeTruthy();
  });

  // The reader's own mailbox took delivery too, so this thread reached them.
  // Answering your own mail is not answering somebody else's.
  it("says nothing when the reader's own mailbox took delivery", async () => {
    await openOnColleaguesThread(["u-charlotte", "u-lars"]);
    await screen.findByRole("button", { name: "Draft with AI" });
    expect(screen.queryByText(/mailbox/)).toBeNull();
  });

  // A hand-logged activity was typed by somebody rather than delivered to
  // anybody. An empty list is an answer, and the answer is "nobody's".
  it("says nothing when no mailbox took delivery", async () => {
    await openOnColleaguesThread([]);
    await screen.findByRole("button", { name: "Draft with AI" });
    expect(screen.queryByText(/mailbox/)).toBeNull();
  });

  // Several colleagues, joined by the locale's own conjunction rather than a
  // comma list an i18n key would have to spell per language.
  it("names every colleague when a thread reached more than one", async () => {
    stubRoutes({
      ...mailboxRoutes(["u-charlotte", "u-mara"]),
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u-charlotte",
              display_name: "Charlotte Weber",
              email: "c@demo.test",
            },
            { id: "u-mara", display_name: "Mara Ott", email: "m@demo.test" },
            { id: "u-lars", display_name: "Lars", email: "l@demo.test" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /ACTION REQUIRED/ }),
    );
    expect(
      await screen.findByText(/Charlotte Weber and Mara Ott/),
    ).toBeTruthy();
  });

  // A seat the roster cannot name still gets a complete sentence. The label is
  // a whole noun phrase, never a fragment the possessive would mangle.
  it("falls back to a complete label for a seat the roster cannot name", async () => {
    stubRoutes({
      ...mailboxRoutes(["u-ghost"]),
      "GET /users": () =>
        jsonResponse({
          data: [{ id: "u-lars", display_name: "Lars", email: "l@demo.test" }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /ACTION REQUIRED/ }),
    );
    const notice = await screen.findByText(/a colleague's mailbox/);
    expect(notice).toBeTruthy();
    // Never the raw id, and never the doubled possessive a bare fragment makes.
    expect(notice.textContent).not.toMatch(/u-ghost/);
    expect(notice.textContent).not.toMatch(/'s's/);
  });

  // Until the viewer is known, every thread would read as somebody else's.
  it("says nothing while the reader is not yet known", async () => {
    stubRoutes({
      ...mailboxRoutes(["u-charlotte"]),
      "GET /me": () => emptyResponse(401),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /ACTION REQUIRED/ }),
    );
    await screen.findByRole("button", { name: "Draft with AI" });
    expect(screen.queryByText(/mailbox/)).toBeNull();
  });

  // Switching threads must not leave the previous thread's notice on screen.
  // React Query serves the settled entry for the OLD key while the new one
  // loads, so a notice read straight off `data` names the wrong colleague for
  // as long as the second request takes.
  it("drops the previous thread's notice while the next one loads", async () => {
    const mine: Activity = {
      ...colleaguesMail,
      id: "act-mine",
      subject: "My own thread",
      captured_by: "connector:gmail:u-lars",
    };
    let release: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    stubRoutes({
      ...mailboxRoutes(["u-charlotte"]),
      "GET /activities/act-mine": () => jsonResponse(mine),
      // The second thread's answer is withheld until the assertion has run,
      // which is the window a stale notice would be visible in.
      "GET /activities/act-mine/reply-recipient": async () => {
        await held;
        return jsonResponse({
          full_name: "Accounts Singapore",
          first_name: "Accounts",
          address: "accounts@vendor.test",
          mailbox_user_ids: ["u-lars"],
        });
      },
    });
    const view = render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        activityId="act-charlotte"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByText(/Charlotte Weber's mailbox/);

    // Re-anchor on the reader's OWN thread, whose answer has not arrived yet.
    // Charlotte's name must go at once rather than waiting for it.
    view.rerender(
      <QueryClientProvider client={view.queryClient}>
        <LocaleProvider initial="en">
          <ComposeModal
            entityType="company"
            entityId="company-1"
            activityId="act-mine"
            open
            onClose={vi.fn()}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await waitFor(() =>
      expect(screen.queryByText(/Charlotte Weber/)).toBeNull(),
    );
    release?.();
  });

  // An older server that does not send the field at all must not be read as
  // "a colleague's", nor crash the composer.
  it("says nothing when the server does not send the field", async () => {
    stubRoutes({
      ...mailboxRoutes([]),
      "GET /activities/act-charlotte/reply-recipient": () =>
        jsonResponse({
          full_name: "Accounts Singapore",
          first_name: "Accounts",
          address: "accounts@vendor.test",
        }),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", { name: /ACTION REQUIRED/ }),
    );
    await screen.findByRole("button", { name: "Draft with AI" });
    expect(screen.queryByText(/mailbox/)).toBeNull();
  });
});
