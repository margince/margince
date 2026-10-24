import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { writeMessage } from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "../../testing/sendpermission.testkit";

// The composer's "why are you writing?" dial is found by name, not by role
// alone. To, Cc and Bcc are comboboxes too, so a bare role query matches four
// controls and every readiness wait has to say which.
export const WHY_ASK = "Reason for contact";

export const DISCLOSURE = { name: "AI-assisted draft" };

export type Activity = components["schemas"]["Activity"];

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// A 501 answer carries no JSON body when the mailer or model is not wired.
// The composer must branch on the raw status, not on a parsed problem.
export function emptyResponse(status: number) {
  return new Response(null, { status });
}

export function problemResponse(body: unknown, status: number) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

export function render(
  ui: ReactNode,
): RenderResult & { queryClient: QueryClient } {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // The client is returned so a test can re-render against the same cache. A
  // fresh one would empty it, which is the opposite of the stale-entry case.
  return {
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
    queryClient: client,
  };
}

// Two purposes, because the send rules differ by purpose: transactional is the
// one locked, unsubscribe-free lane; anything else renders a per-recipient
// unsubscribe link.
export const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
    {
      id: "p2",
      key: "marketing_email",
      label: "Marketing email",
      requires_double_opt_in: true,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// listVoiceProfiles caps at one profile and answers an empty page when the
// caller has none. Most composer tests run in that state.
export const NO_VOICE_PROFILE = {
  data: [],
  page: { next_cursor: null, has_more: false },
};

// One profile whose maturity is the middle band (800–4000 corpus words): enough
// to style a draft, not yet a full build.
export const PROVISIONAL_VOICE_PROFILE = {
  data: [
    {
      id: "vp-1",
      owner_id: "u1",
      status: "ready",
      maturity: "provisional",
      quality_band: "thin",
      voice_profile_md: "Short sentences.",
      profile_version: 3,
      personality_md: "",
      auto_learning_enabled: false,
      active_source_hash: null,
      candidate_version: null,
      last_built_at: null,
      source: "manual",
      captured_by: "human:u1",
      version: 1,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
      archived_at: null,
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// Records every request so a test can assert what went to the server. The
// request body and headers are the contract for a send or relink.
export type Sent = {
  key: string;
  body: unknown;
  headers: Headers;
  query: URLSearchParams;
};

export function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read and write the composer makes; its branches are the route table
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      const headers = request
        ? request.headers
        : new Headers(init?.headers ?? {});
      sent.push({ key, body, headers, query: url.searchParams });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse(NO_VOICE_PROFILE);
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({});
    }),
  );
  return sent;
}

export const activity202: Activity = {
  offers_email: false,
  id: "act-1",
  kind: "email",
  subject: "Re: Q3",
  occurred_at: "2026-07-01T00:00:00Z",
  is_done: false,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
};

// What a rep answers when the composer asks why they are writing. These are
// the categories the engine reasons about, not consent purposes. The record
// decides what is allowed, and a key a rep picked never did.
export const WHY_LABEL = {
  requestedFollowup: "Follow-up they requested",
  marketing: "Marketing",
} as const;

// The composer has one dropdown, so a pick needs nothing but the label.
// `pickOption` takes a userEvent session, which the bare direct API is not.
// So it gets a fresh one, as every bare `userEvent.*` call here does inside.
export function pickWhy(label: string) {
  return pickOption(
    userEvent.setup(),
    screen.getByRole("combobox", { name: WHY_ASK }),
    label,
  );
}

// Fills the Send preconditions so a test can exercise the outcome under study.
//
// The "why" answer is filled because these suites compose an unanchored
// message. No anchor read is stubbed, so the composer has nothing to derive
// from and asks. A suite whose subject is a reply stubs the anchor instead, and
// the question never appears. Asking there is the defect fillReplyForm catches.
export async function fillSendableForm() {
  await userEvent.type(screen.getByLabelText("To"), "a@x.com");
  await userEvent.tab();
  await userEvent.type(screen.getByLabelText("Subject"), "Hi there");
  writeMessage("Body", "Body content");
  await pickWhy(WHY_LABEL.requestedFollowup);
}

// The threads the composer offers sit in a menu behind the bar above the
// fields. Finds one by name, opening the menu first when it is shut.
export async function findThreadChoice(name: string | RegExp) {
  const shown = screen.queryByRole("button", { name });
  if (shown) {
    return shown;
  }
  await userEvent.click(
    await screen.findByRole("button", { name: /(Continue a thread|Change)$/ }),
  );
  return screen.findByRole("button", { name });
}
