/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import { allowedPreview, isPreviewDoor } from "./sendpermission.testkit";

// The composer, opened onto a message that is no longer there.
//
// A scan's advice is written once and replayed on every open, so the rep can
// press "Create draft" on a card citing a message they archived afterwards.
// GET /activities/{id} still answers 200 for an archived row, so the composer
// opens as if all were well; the draft endpoint reads the anchor live and is
// the first refusal the rep meets. Its bare "not found" named neither the
// message nor what they should do instead.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

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
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (isPreviewDoor(url.pathname)) return jsonResponse(allowedPreview([]));
      return jsonResponse({});
    }),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("drafting a reply to a message that is gone", () => {
  it("names the vanished anchor instead of the server's bare not-found", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({ title: "not found", code: "not_found" }, 404),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /Draft (reply )?with AI/ }),
    );

    expect(await screen.findByText(/no longer available/i)).toBeTruthy();
    expect(screen.queryByText("not found")).toBeNull();
  });

  // A composer with NO anchor 404s when the RECORD is gone, not a message.
  // The same draft mutation serves contact, company and lead composers, and
  // telling a rep writing a fresh mail that there is nothing to reply to would
  // name the wrong record and advise what they are already doing.
  it("does not blame a missing anchor when there is no anchor", async () => {
    stubRoutes({
      "POST /contacts/c-1/draft-email": () =>
        jsonResponse(
          {
            title: "not found",
            detail: "That contact is no longer on file.",
            code: "not_found",
          },
          404,
        ),
    });
    render(
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        recordAddress="annabelle@akeneo.example"
        open
        onClose={vi.fn()}
      />,
    );

    // The fresh-mail composer will not draft without a purpose, so the click
    // below would otherwise be a no-op and this test would pass vacuously.
    await userEvent.type(
      screen.getByPlaceholderText(/Purpose of the email|Purpose of the reply/),
      "Ask about the rollout",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /Draft (reply )?with AI/ }),
    );

    expect(
      await screen.findByText("That contact is no longer on file."),
    ).toBeTruthy();
    expect(screen.queryByText(/nothing to reply to/i)).toBeNull();
  });

  // A drafting failure that is NOT the anchor keeps its own copy. Answering
  // every failure with "that message is gone" would tell a rep to write a new
  // message when the one thing wrong was a lane being down for a minute.
  it("does not claim the message is gone for an unrelated failure", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(
          {
            title: "the draft could not be written",
            detail: "The drafting model did not answer.",
            code: "upstream_failure",
          },
          502,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: /Draft (reply )?with AI/ }),
    );

    // The shared line for a failure with no reader-facing cause, which is
    // what a 502 is. What matters is the branch it is NOT on.
    expect(await screen.findByText(/did not finish the request/i)).toBeTruthy();
    expect(screen.queryByText(/no longer available/i)).toBeNull();
  });
});
