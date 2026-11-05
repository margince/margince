import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { renderWithQueryClient as render } from "../../testing/queryrender";
import { jsonResponse } from "../../testing/story-utils";

export { jsonResponse, render };

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
      label: "Marketing",
      requires_double_opt_in: true,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// Records every request so a test can check what went to the server. The
// request body is the contract for a purpose write. The harness has the shape
// of the one in consent.test.tsx; each test file keeps its own copy.
export type Sent = { key: string; url: string; body: unknown };

export function stubRoutes(
  overrides: Record<string, () => Response> = {},
  sent: Sent[] = [],
) {
  vi.stubGlobal(
    "fetch",
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every privacy read and write; its branches are the route table
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
            ? await request.json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, url: url.pathname + url.search, body });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      // Falls back to DSRS, so every PrivacyInboxCard test gets seed rows
      // without its own override. DSRS is declared below, but the module has
      // finished loading before any `it` callback runs.
      if (key === "GET /data-subject-requests") return jsonResponse(DSRS);
      if (key === "GET /data-subject-requests/deadline")
        return jsonResponse(COMPUTED_DEADLINE);
      if (key === "GET /users" || key === "GET /confirm-submissions")
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      // The FOUR grants this screen's cards ask for, because they are four
      // different questions and the server asks each separately:
      //   privacy_request:read   — the subject queue (consent/dsr.go)
      //   privacy_request:update — moving a request through its statuses
      //   contact:update          — OPENING one, which writes the contact named
      //   consent_config:create  — appending to the consent registry
      // The default principal holds all four. A test asserting any one refusal
      // overrides this key with the narrower set.
      if (key === "GET /me")
        return jsonResponse(
          meFixture({
            roles: ["admin"],
            allow: {
              privacy_request: ["read", "update"],
              contact: ["update"],
              consent_config: ["create"],
            },
          }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

// Drives one of this screen's dropdowns by the label a reader sees. `pickOption`
// needs a userEvent session, so it gets a fresh one. Every bare `userEvent.*`
// call does the same inside.
export function choose(control: HTMLElement, optionLabel: string) {
  return pickOption(userEvent.setup(), control, optionLabel);
}

// Writes what a close needs: the answer, and how the identity was checked.
export async function draftClose(answer: string) {
  await userEvent.type(screen.getByLabelText(/resolution/i), answer);
  await choose(
    screen.getByLabelText(en["privacy.identityCheck"]),
    en["privacy.identityCheck.id_document"],
  );
}

export const DSRS = {
  data: [
    {
      id: "d1",
      kind: "erasure",
      subject_ref: "8f3a-contact-uuid",
      status: "open",
      due_at: "2026-08-01T00:00:00Z",
      due_overridden: false,
      dispute_open: false,
      created_at: "2026-07-01T00:00:00Z",
    },
    {
      id: "d2",
      kind: "access",
      subject_ref: "anna@acme.test",
      status: "fulfilled",
      resolution: "sent by post",
      due_at: "2026-07-12T00:00:00Z",
      due_overridden: false,
      dispute_open: false,
      created_at: "2026-06-01T00:00:00Z",
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// The facet bar's status words are the transition verbs' words too. So a query
// scopes to the request: its open drawer, or else its table row.
export async function findDsrRow(subjectRef: string) {
  const drawer = screen.queryByRole("dialog", { name: subjectRef });
  if (drawer) {
    return drawer;
  }
  const row = (await screen.findByText(subjectRef)).closest("tr");
  if (!(row instanceof HTMLElement)) {
    throw new Error(`dsr row for "${subjectRef}" not found`);
  }
  return row;
}

// What the server computes for a request filed now: the GDPR month.
export const COMPUTED_DEADLINE = {
  due_at: "2026-08-15T14:00:00Z",
  rule: "gdpr",
};
