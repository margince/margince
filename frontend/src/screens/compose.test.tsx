/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { ComposeModal, RelinkModal, TimelineActions } from "./compose";

type Activity = components["schemas"]["Activity"];

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// A 501 answer carries no JSON body (the mailer/model is simply not wired), so
// the composer must branch on the raw status, not on a parsed problem.
function emptyResponse(status: number) {
  return new Response(null, { status });
}

function problemResponse(body: unknown, status: number) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/problem+json" },
  });
}

// The 422 body httperr.Validation actually emits. Every validation problem
// carries the same top-level `code`, so the rule that fired is named only in
// details.errors[] beside the field it was asserted about — a stub that
// hoisted the specific code to the top level would prove the screen reads a
// shape the server never sends.
function validationProblem(field: string, code: string, message: string) {
  return {
    code: "validation_error",
    title: "Unprocessable Entity",
    detail: message,
    details: { errors: [{ field, code, message }] },
  };
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

// Two purposes, because the send rules differ by purpose: transactional is the
// one locked, unsubscribe-free lane; anything else renders a per-recipient
// unsubscribe link.
const PURPOSES = {
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
// caller has none — the state most composer tests run in.
const NO_VOICE_PROFILE = {
  data: [],
  page: { next_cursor: null, has_more: false },
};

// One profile whose maturity is the middle band (800–4000 corpus words): enough
// to style a draft, not yet a full build.
const PROVISIONAL_VOICE_PROFILE = {
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

// rejectVoiceDraft answers the owner's updated learning aggregate; the composer
// only needs the call to have succeeded.
const LEARNING_SUMMARY = {
  drafted: 4,
  accepted: 1,
  edited_sent: 2,
  rejected: 1,
  qualifying_source_count: 0,
  qualifying_words: 0,
  transformations: [],
};

// Records every request so a test can assert what actually went to the server
// — the request body and headers ARE the contract for a send/relink.
type Sent = { key: string; body: unknown; headers: Headers };

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
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
      sent.push({ key, body, headers });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse(NO_VOICE_PROFILE);
      return jsonResponse({});
    }),
  );
  return sent;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const activity202: Activity = {
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

describe("RelinkModal", () => {
  it("relinks the search-picked target and closes on 200", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "deal", id: "d-9", title: "Acme renewal" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Acme");
    const candidate = await screen.findByRole("button", {
      name: "Acme renewal",
    });
    await userEvent.click(candidate);
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const relink = sent.find((r) => r.key === "POST /activities/act-1/relink");
    expect(relink?.body).toEqual({
      entity_type: "deal",
      entity_id: "d-9",
      replace_existing_of_type: false,
    });
    // Relink is idempotency-keyed (its no-dup-on-replay contract).
    expect(relink?.headers.get("Idempotency-Key")).toBeTruthy();
    // AND it carries the version this reader saw. The key and the precondition
    // are one header slot, so a regression that wrote them separately would
    // send whichever came last — the key alone, silently unconditioned.
    expect(relink?.headers.get("If-Match")).toBe("4");
  });

  // A copy read without a version cannot say what it is changing, so the relink
  // is refused BY NAME rather than through requireVersion's bare throw — which
  // reached the reader as the generic "something went wrong", on the very error
  // path the write leans on to say it did not go through.
  it("refuses in words when the activity was read without a version", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "deal", id: "d-9", title: "Acme renewal" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={null}
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Acme");
    await userEvent.click(
      await screen.findByRole("button", { name: "Acme renewal" }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    expect(await screen.findByText(/read without a version/i)).toBeTruthy();
    // And nothing was sent: a refusal that still wrote would be the
    // last-write-wins this whole change is about.
    expect(sent.find((r) => r.key === "POST /activities/act-1/relink")).toBe(
      undefined,
    );
    expect(onClose).not.toHaveBeenCalled();
  });

  it("sends replace_existing_of_type when the move toggle is on", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "organization", id: "o-2", title: "Globex" }],
          page: { has_more: false },
        }),
      "POST /activities/act-1/relink": () => jsonResponse(activity202),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="deal"
        entityId="d-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "Globex");
    await userEvent.click(
      await screen.findByRole("button", { name: "Globex" }),
    );
    await userEvent.click(screen.getByRole("checkbox"));
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const relink = sent.find((r) => r.key === "POST /activities/act-1/relink");
    expect(relink?.body).toEqual({
      entity_type: "organization",
      entity_id: "o-2",
      replace_existing_of_type: true,
    });
    expect(relink?.headers.get("If-Match")).toBe("4");
  });

  it("moves the whole conversation through relinkThread when asked", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [{ type: "project", id: "pr-4", title: "ERP rollout" }],
          page: { has_more: false },
        }),
      "POST /activities/relink-thread": () => jsonResponse({ relinked: 3 }),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        threadKey="thread:abc"
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "ERP");
    await userEvent.click(
      await screen.findByRole("button", { name: "ERP rollout" }),
    );
    await userEvent.click(
      screen.getByRole("checkbox", {
        name: "Also move the rest of this conversation",
      }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Relink" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const thread = sent.find((r) => r.key === "POST /activities/relink-thread");
    expect(thread?.body).toEqual({
      thread_key: "thread:abc",
      entity_type: "project",
      entity_id: "pr-4",
      replace_existing_of_type: false,
    });
    expect(thread?.headers.get("Idempotency-Key")).toBeTruthy();
    // And NO precondition: a thread moves many activities, and one version
    // cannot condition them — the server refuses a pinned batch by name.
    expect(thread?.headers.get("If-Match")).toBeNull();
    // The single-activity route is not called on the way.
    expect(sent.find((r) => r.key === "POST /activities/act-1/relink")).toBe(
      undefined,
    );
  });

  it("offers the conversation toggle only when the activity has a thread", () => {
    stubRoutes({});
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );
    expect(
      screen.queryByRole("checkbox", {
        name: "Also move the rest of this conversation",
      }),
    ).toBeNull();
  });

  it("drops activity results — relink has no activity target", async () => {
    stubRoutes({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "activity", id: "a-x", title: "Some email" },
            { type: "person", id: "pp-1", title: "Jane Doe" },
          ],
          page: { has_more: false },
        }),
    });
    render(
      <RelinkModal
        activityId="act-1"
        activityVersion={4}
        entityType="deal"
        entityId="d-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(screen.getByRole("searchbox"), "e");
    expect(
      await screen.findByRole("button", { name: "Jane Doe" }),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Some email" })).toBeNull();
  });
});

// The two purposes as the rep READS them: a pick names what a person would
// click, while the ConsentPurpose.key each label stands for is the wire value,
// asserted on the request body wherever a send is under study.
const PURPOSE_LABEL = {
  transactional: "Deal messages",
  marketing: "Marketing email",
} as const;

// The composer has exactly one dropdown, so a pick needs nothing but the label.
// `pickOption` takes a userEvent SESSION, which the bare direct API is not, so
// it gets a fresh one — the same thing every bare `userEvent.*` call in this
// file does internally.
function pickPurpose(label: string) {
  return pickOption(userEvent.setup(), screen.getByRole("combobox"), label);
}

// Fills the four Send preconditions (To, subject, body, purpose) so a test can
// then exercise the send outcome under study.
async function fillSendableForm() {
  await userEvent.type(screen.getByLabelText("To"), "a@x.com");
  await userEvent.tab();
  await userEvent.type(screen.getByPlaceholderText("Subject"), "Hi there");
  await userEvent.type(screen.getByPlaceholderText("Body"), "Body content");
  await pickPurpose(PURPOSE_LABEL.transactional);
}

describe("ComposeModal", () => {
  it("fills To/Subject/Body from the AI draft", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          to: ["buyer@acme.test"],
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    // getByDisplayValue reads the field's current value without a DOM cast.
    expect(await screen.findByDisplayValue("Re: Q3 numbers")).toBeTruthy();
    expect(screen.getByDisplayValue("Thanks for the note.")).toBeTruthy();
    // EmailDraft.to prefills the recipient chips.
    expect(screen.getByText("buyer@acme.test")).toBeTruthy();
  });

  // Art. 50 is a hard gate: a model-produced draft that reaches a human
  // without a disclosure is a compliance failure, so these three cases fix the
  // banner's presence, its verbatim text, and its absence on human-written
  // text. Removing the banner from the composer fails all three.
  it("discloses a model-produced draft, rendering the server's line verbatim", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "AI-assisted draft (Art. 50): reviewed by a human.",
          draft_ref: null,
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByTestId("ai-disclosure-banner")).toBeTruthy();
    expect(
      screen.getByText("AI-assisted draft (Art. 50): reviewed by a human."),
    ).toBeTruthy();
  });

  it("still discloses when the server sends no disclosure line", async () => {
    // ai_disclosure is contract-guaranteed alongside ai_generated, but a
    // client that trusts that would drop the disclosure entirely against an
    // older or misbehaving server. Absence of the line is not absence of the
    // obligation, so the composer carries its own wording.
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: null,
          draft_ref: null,
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByTestId("ai-disclosure-banner")).toBeTruthy();
    expect(screen.getByText(/This draft was produced by AI/i)).toBeTruthy();
  });

  it("discloses nothing when no model produced the draft", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: false,
          ai_disclosure: null,
          draft_ref: null,
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    // The fill proves the draft landed, so the missing banner is the
    // disclosure being conditional rather than the response never arriving.
    expect(await screen.findByDisplayValue("Re: Q3 numbers")).toBeTruthy();
    expect(screen.queryByTestId("ai-disclosure-banner")).toBeNull();
  });

  it("names the voice version that styled the draft and flags a provisional profile", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "AI-assisted draft (Art. 50).",
          voice_profile_version: 3,
          draft_ref: "vd-1",
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByText("Built from your corpus · v3")).toBeTruthy();
    expect(screen.getByText("Provisional voice")).toBeTruthy();
  });

  it("flags nothing provisional when the profile is past that band", async () => {
    stubRoutes({
      "GET /voice-profiles": () =>
        jsonResponse({
          ...PROVISIONAL_VOICE_PROFILE,
          data: [
            { ...PROVISIONAL_VOICE_PROFILE.data[0], maturity: "building" },
          ],
        }),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          subject: "Re: Q3 numbers",
          body: "Thanks for the note.",
          ai_generated: true,
          ai_disclosure: "AI-assisted draft (Art. 50).",
          voice_profile_version: 3,
          draft_ref: "vd-1",
        }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByText("Built from your corpus · v3")).toBeTruthy();
    expect(screen.queryByText("Provisional voice")).toBeNull();
  });

  it("shows an unavailable note on a 501 draft, keeping the form usable", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () => emptyResponse(501),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByText(/AI drafting is unavailable/i)).toBeTruthy();
    // Manual composing still works — Send is present.
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
  });

  it("keeps Send disabled until To, subject, body, and purpose are set", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");

    expect(
      screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
    ).toBe(true);
    await fillSendableForm();
    expect(
      screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
    ).toBe(false);
  });

  it("sends the edited email with no approval token or idempotency key", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    // Nothing was drafted, so no voice-learning outcome may be inferred: the
    // exact-match assertion is what keeps `draft_ref` off an independently
    // composed send rather than riding along as null.
    expect(req?.body).toEqual({
      subject: "Hi there",
      body: "Body content",
      to: ["a@x.com"],
      consent_purpose: "transactional",
    });
    // ADR-0055: the human click is the approval — neither header rides along.
    expect(req?.headers.get("X-Approval-Token")).toBeNull();
    expect(req?.headers.get("Idempotency-Key")).toBeNull();
  });

  it("surfaces the default-deny consent gate on 409 without closing", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          {
            code: "consent_not_granted",
            detail: "suppressed",
            title: "Conflict",
          },
          409,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        personId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/has not granted consent/i)).toBeTruthy();
    // The gate points at the consent surface, and the modal stays open.
    expect(screen.getByRole("link", { name: "Review consent" })).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a sending-unavailable note on a 501 send, not an error", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () => emptyResponse(501),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/Sending is unavailable/i)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("does not report a send on a bodiless non-2xx (gateway 5xx)", async () => {
    const onClose = vi.fn();
    // openapi-fetch yields a falsy error for a bodiless response; success must
    // be the real 202, so an empty 503 stays an error and keeps the modal open
    // rather than falsely confirming an irreversible send.
    stubRoutes({
      "POST /activities/act-1/send-email": () => emptyResponse(503),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/The request failed/i)).toBeTruthy();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a draft error on a bodiless non-2xx without crashing the fill", async () => {
    // The old !error success path would fabricate an undefined draft and crash
    // onSuccess reading its fields; a bodiless 502 must surface an error only.
    //
    // On THIS route the error says what happened: a draft runs for tens of
    // seconds, so a proxy giving up on one leaves work in flight, and a retry
    // is a second model call rather than a repeat. Every other route keeps the
    // generic sentence — see api/client.test.ts.
    stubRoutes({
      "POST /activities/act-1/draft-email": () => emptyResponse(502),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(
      await screen.findByText(/did not finish this request in time/i),
    ).toBeTruthy();
    // And it warns before a retry, because the first call may still be running.
    expect(screen.getByText(/may still be working/i)).toBeTruthy();
  });
});

// A voice-styled draft, served with the reference the server keys a learning
// outcome against and the profile version that styled it.
function voiceDraft(
  ref: string,
  subject: string,
  body: string,
  voiceVersion = 3,
) {
  return {
    subject,
    body,
    to: ["buyer@acme.test"],
    ai_generated: true,
    ai_disclosure: "AI-assisted draft (Art. 50).",
    voice_profile_version: voiceVersion,
    draft_ref: ref,
  };
}

// Serves a different draft per call, so a test can re-draft and see which of
// the two references the composer is still holding.
function draftsInTurn(...drafts: object[]) {
  let call = 0;
  return () => {
    const drafted = drafts[Math.min(call, drafts.length - 1)];
    call += 1;
    return jsonResponse(drafted);
  };
}

function renderComposer(onClose = vi.fn()) {
  render(
    <ComposeModal
      activityId="act-1"
      entityType="person"
      entityId="p-1"
      open
      onClose={onClose}
    />,
  );
  return onClose;
}

// The send-time binding to the voice draft: `draft_ref` is what makes the
// server's accepted/edited_sent verdict about the rep's own words.
describe("ComposeModal draft binding", () => {
  it("sends the reference of the draft whose text it is sending", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({
      body: "Draft A body.",
      draft_ref: "vd-a",
    });
  });

  it("keeps the earlier reference when a re-draft leaves the typed body alone", async () => {
    // The fill rule never clobbers text the rep has touched, so the second
    // draft's words are discarded. Adopting its reference anyway would submit
    // draft B's identity with draft A's text plus the rep's edit — an
    // edited_sent whose "edit" is a draft the rep never saw.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body."),
        voiceDraft("vd-b", "Re: Q3 again", "Draft B body."),
      ),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    const bodyField = await screen.findByDisplayValue("Draft A body.");
    await userEvent.type(bodyField, " And my own line.");
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")
          .length,
      ).toBe(2),
    );

    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({
      body: "Draft A body. And my own line.",
      draft_ref: "vd-a",
    });
  });

  it("drops the reference once the body it named is cleared", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    const bodyField = await screen.findByDisplayValue("Draft A body.");
    await userEvent.clear(bodyField);
    await userEvent.type(bodyField, "Written from scratch.");
    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).not.toHaveProperty("draft_ref");
  });

  it("records exactly one rejection when the rep discards the draft", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        jsonResponse(LEARNING_SUMMARY),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    await waitFor(() =>
      expect(screen.queryByDisplayValue("Draft A body.")).toBeNull(),
    );
    const rejections = sent.filter(
      (r) => r.key === "POST /voice-profiles/vp-1/draft-rejections",
    );
    expect(rejections).toHaveLength(1);
    expect(rejections[0].body).toEqual({ draft_ref: "vd-a" });
  });

  it("bars a send while a rejection of the same draft is in flight", async () => {
    // A rejection and a send are contradictory verdicts on one draft and the
    // signal keeps whichever lands first. A send that slipped through here
    // would leave the durable learning record saying "rejected" for a message
    // that actually went out — the exact failure the explicit judgment exists
    // to prevent.
    let landRejection = (): void => {};
    const rejectionInFlight = new Promise<void>((resolve) => {
      landRejection = resolve;
    });
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        rejectionInFlight.then(() => jsonResponse(LEARNING_SUMMARY)),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await pickPurpose(PURPOSE_LABEL.transactional);
    // The form is sendable before the judgment starts, so the refusal below is
    // the rejection holding the draft rather than an unmet precondition.
    expect(
      screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
    ).toBe(false);

    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Send" }).hasAttribute("disabled"),
      ).toBe(true),
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);

    landRejection();
    await waitFor(() =>
      expect(screen.queryByDisplayValue("Draft A body.")).toBeNull(),
    );
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);
  });

  it("hands the reference back to the send when the rejection fails", async () => {
    // A rejection that never landed left the signal open, so the draft on
    // screen is still the one it named: the send that follows must still be
    // able to report what the rep did with those words.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        problemResponse(
          { code: "internal_error", title: "Server Error", detail: "boom" },
          500,
        ),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    expect(await screen.findByText(/boom/i)).toBeTruthy();

    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("keeps the draft when a bodiless gateway failure refuses the rejection", async () => {
    // openapi-fetch reports a falsy `error` and no `data` for a bodiless
    // non-2xx, so a rejection that never reached the server looks exactly like
    // one that succeeded. Clearing the composer on it would tell the rep their
    // verdict was recorded while the signal is still open — and the next send
    // would then be classified against a draft the rep believes they discarded.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () => emptyResponse(502),
      "POST /activities/act-1/send-email": () => jsonResponse(activity202, 202),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    expect(
      await screen.findByText("The request failed. Please try again."),
    ).toBeTruthy();
    expect(screen.getByDisplayValue("Draft A body.")).toBeTruthy();

    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() =>
      expect(
        sent.some((r) => r.key === "POST /activities/act-1/send-email"),
      ).toBe(true),
    );
    const req = sent.find((r) => r.key === "POST /activities/act-1/send-email");
    expect(req?.body).toMatchObject({ draft_ref: "vd-a" });
  });

  it("announces a failed rejection rather than only colouring it", async () => {
    // The line appears without any navigation, so a rep who cannot see it is
    // otherwise never told the judgment did not land.
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        problemResponse(
          { code: "internal_error", title: "Server Error", detail: "boom" },
          500,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("boom");
  });

  it("announces a failed draft rather than only colouring it", async () => {
    stubRoutes({
      "POST /activities/act-1/draft-email": () =>
        problemResponse(
          { code: "internal_error", title: "Server Error", detail: "no model" },
          500,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("no model");
  });

  it("freezes the drafted text while the rejection is in flight", async () => {
    // A failed rejection hands its reference back to the surface. If the rep
    // could type meanwhile, that reference would return over words it does not
    // describe, and a later send would file an outcome against a draft nobody
    // wrote — the same defect as adopting a reference whose body was never
    // applied.
    let landRejection = (): void => {};
    const rejectionInFlight = new Promise<void>((resolve) => {
      landRejection = resolve;
    });
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
      "POST /voice-profiles/vp-1/draft-rejections": () =>
        rejectionInFlight.then(() => jsonResponse(LEARNING_SUMMARY)),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    const bodyField = await screen.findByDisplayValue("Draft A body.");
    const subjectField = screen.getByDisplayValue("Re: Q3");
    expect(bodyField.hasAttribute("disabled")).toBe(false);

    await userEvent.click(
      screen.getByRole("button", { name: "Discard draft" }),
    );
    await waitFor(() => expect(bodyField.hasAttribute("disabled")).toBe(true));
    expect(subjectField.hasAttribute("disabled")).toBe(true);
    await userEvent.type(bodyField, " and mine");
    expect(screen.getByDisplayValue("Draft A body.")).toBeTruthy();

    landRejection();
    await waitFor(() =>
      expect(screen.queryByDisplayValue("Draft A body.")).toBeNull(),
    );
  });

  it("records no rejection when the composer is merely closed", async () => {
    // `rejected` is a judgment, not an accident of navigation — and because the
    // reference is deterministic and the drafted signal inserts once, a
    // rejection logged on a close would stand in for the real outcome of an
    // identical draft that is later sent.
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    const onClose = renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByDisplayValue("Draft A body.");
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onClose).toHaveBeenCalled();
    expect(sent.some((r) => r.key.includes("draft-rejections"))).toBe(false);
  });
});

// The Art. 50 banner describes the words on screen, so it rides on exactly the
// condition that puts a served draft there and leaves when they do. A banner
// that outlived its text would attribute a human's writing to a model, or
// credit it to a voice version that never touched it.
describe("ComposeModal draft provenance", () => {
  it("discloses nothing when a draft's words were discarded for the rep's own", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.type(screen.getByPlaceholderText("Body"), "My own words.");
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    // The subject fill proves the draft landed, so the missing banner is the
    // disclosure following the body rather than the response never arriving.
    expect(await screen.findByDisplayValue("Re: Q3")).toBeTruthy();
    expect(screen.getByDisplayValue("My own words.")).toBeTruthy();
    expect(screen.queryByTestId("ai-disclosure-banner")).toBeNull();
  });

  it("keeps the applied draft's voice version when a re-draft is discarded", async () => {
    const sent = stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": draftsInTurn(
        voiceDraft("vd-a", "Re: Q3", "Draft A body.", 3),
        voiceDraft("vd-b", "Re: Q3 again", "Draft B body.", 4),
      ),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    const bodyField = await screen.findByDisplayValue("Draft A body.");
    await userEvent.type(bodyField, " And my own line.");
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await waitFor(() =>
      expect(
        sent.filter((r) => r.key === "POST /activities/act-1/draft-email")
          .length,
      ).toBe(2),
    );

    // v4's words were never applied, so v4 may not be named over v3's.
    expect(screen.getByText("Built from your corpus · v3")).toBeTruthy();
    expect(screen.queryByText("Built from your corpus · v4")).toBeNull();
  });

  it("drops the disclosure once the body it described is cleared", async () => {
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse(voiceDraft("vd-a", "Re: Q3", "Draft A body.")),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    const bodyField = await screen.findByDisplayValue("Draft A body.");
    expect(screen.getByTestId("ai-disclosure-banner")).toBeTruthy();

    await userEvent.clear(bodyField);

    await waitFor(() =>
      expect(screen.queryByTestId("ai-disclosure-banner")).toBeNull(),
    );
  });

  it("claims no voice maturity on a draft no voice profile styled", async () => {
    // Maturity is a corpus-word band, so it reads `provisional` while the
    // profile is still only collecting — a state in which drafting serves a
    // plain model draft and the served version is null. The provisional copy
    // says a voice already shapes the draft, which here nothing does.
    stubRoutes({
      "GET /voice-profiles": () => jsonResponse(PROVISIONAL_VOICE_PROFILE),
      "POST /activities/act-1/draft-email": () =>
        jsonResponse({
          ...voiceDraft("vd-a", "Re: Q3", "Draft A body."),
          voice_profile_version: null,
        }),
    });
    renderComposer();
    await screen.findByRole("combobox");

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    expect(await screen.findByTestId("ai-disclosure-banner")).toBeTruthy();
    expect(screen.queryByText("Provisional voice")).toBeNull();
  });
});

// The send pre-flight's two refusals: each is a product state with a fix, so
// each must read as its own copy rather than as the generic failure line the
// server's detail string would otherwise land in.
describe("ComposeModal send refusals", () => {
  it("tells the rep to reconnect a capture-only mailbox, and where", async () => {
    const onClose = vi.fn();
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "from",
            "mailbox_not_send_capable",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer(onClose);
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText(/never granted permission to send/i),
    ).toBeTruthy();
    expect(
      screen
        .getByRole("link", { name: "Reconnect your mailbox" })
        .getAttribute("href"),
    ).toBe("#/settings/connections");
    // The refusal replaces the generic line rather than joining it.
    expect(screen.queryByText("opaque server wording")).toBeNull();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("states the one-send-per-recipient rule on a shared unsubscribe token", async () => {
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "recipients",
            "shared_unsubscribe_token",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText(/reaches one addressee at a time/i),
    ).toBeTruthy();
    expect(screen.queryByText("opaque server wording")).toBeNull();
  });

  it("keeps the generic line for a refusal it has no copy for", async () => {
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "subject",
            "some_future_refusal",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/opaque server wording/i)).toBeTruthy();
  });

  it("does not read a refusal off the field the server did not assert it about", async () => {
    // The pointed copy answers a specific input: "reconnect your mailbox" is
    // an answer about `from`, and would be the wrong instruction under a later
    // rule that refused some other field. So the pair is matched, not the code
    // alone, and an unrecognised pair keeps the server's own wording.
    stubRoutes({
      "POST /activities/act-1/send-email": () =>
        problemResponse(
          validationProblem(
            "recipients",
            "mailbox_not_send_capable",
            "opaque server wording",
          ),
          422,
        ),
    });
    renderComposer();
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/opaque server wording/i)).toBeTruthy();
    expect(screen.queryByText(/never granted permission to send/i)).toBeNull();
  });

  it("warns about a second addressee before the send, not after it", async () => {
    // Every purpose but transactional renders one recipient's unsubscribe link,
    // so the server refuses a second addressee outright. Saying so after the
    // round trip is strictly worse than saying so while the rep can still fix it.
    const sent = stubRoutes();
    renderComposer();
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.type(screen.getByLabelText("Cc"), "second@x.com");
    await userEvent.tab();

    expect(screen.queryByText(/more than one addressee/i)).toBeNull();
    await pickPurpose(PURPOSE_LABEL.marketing);

    expect(await screen.findByText(/more than one addressee/i)).toBeTruthy();
    // A warning, not a gate — and nothing was sent to earn it.
    expect(
      sent.some((r) => r.key === "POST /activities/act-1/send-email"),
    ).toBe(false);
  });

  it("does not warn about a lone addressee under the same purpose", async () => {
    stubRoutes();
    renderComposer();
    await screen.findByRole("combobox");
    await fillSendableForm();
    await pickPurpose(PURPOSE_LABEL.marketing);

    expect(screen.queryByText(/more than one addressee/i)).toBeNull();
  });
});

// The Telegram reply reuses ComposeModal's confirm-first send, its consent
// rendering, and its post-send refresh wholesale — only the wire target and
// the fields a channel has no concept of differ from the mail path above.
describe("ComposeModal — channel reply", () => {
  const telegramActivity: Activity = {
    ...activity202,
    id: "act-1",
    kind: "message",
    channel_provider: "telegram",
    subject: null,
    direction: "inbound",
  };

  it("posts to send-message for a channel activity", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /activities/act-1/send-message": () =>
        jsonResponse(
          { ...telegramActivity, direction: "outbound", body: "On my way." },
          202,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        kind="message"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await userEvent.type(screen.getByPlaceholderText("Body"), "On my way.");
    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find(
      (r) => r.key === "POST /activities/act-1/send-message",
    );
    // No `to`, `subject`, `cc`, or `draft_ref` — the wire shape a channel send
    // actually accepts (SendMessageRequest carries only these two fields).
    expect(req?.body).toEqual({
      body: "On my way.",
      consent_purpose: "transactional",
    });
    // ADR-0055: the human's own click is the approval on both send paths.
    expect(req?.headers.get("X-Approval-Token")).toBeNull();
    expect(req?.headers.get("Idempotency-Key")).toBeNull();
  });

  it("hides subject and cc for a channel reply", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");
    await userEvent.type(screen.getByPlaceholderText("Body"), "On my way.");

    // The interaction proves the surface is otherwise usable; the missing
    // fields are the point of the test, not an accident of an unrendered form.
    expect(screen.getByDisplayValue("On my way.")).toBeTruthy();
    expect(screen.queryByPlaceholderText("Subject")).toBeNull();
    expect(screen.queryByLabelText("Cc")).toBeNull();
    // A channel has no addressable "to" either — the server resolves the
    // recipient from the conversation's own channel identity (design §9.3).
    expect(screen.queryByLabelText("To")).toBeNull();
  });

  it("names the channel it is about to send on, and says the send is final", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");

    // Confirming an irreversible send under the name of a channel this
    // message will never travel on is a lie the rep cannot check.
    expect(screen.getByText("Send this message?")).toBeTruthy();
    expect(screen.queryByText("Send this email?")).toBeNull();
    // The heading is the only place the channel is named, so it cannot also
    // be the only place the irreversibility is: the modal chrome around it
    // is a tier dot and two buttons.
    expect(screen.getByText(/irreversible action/)).toBeTruthy();
  });

  it("gives the message box an accessible name of its own", async () => {
    stubRoutes();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");

    // getByLabelText resolves labels only — a placeholder does not satisfy
    // it — so this holds the box to a name that survives the rep typing
    // into it, the moment the placeholder disappears.
    expect(screen.getByLabelText("Body")).toBeTruthy();
  });

  it("keeps the drafted text when consent is refused", async () => {
    stubRoutes({
      "POST /activities/act-1/send-message": () =>
        problemResponse(
          {
            code: "consent_not_granted",
            detail: "suppressed",
            title: "Conflict",
          },
          409,
        ),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        personId="p-1"
        kind="message"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");
    await userEvent.type(
      screen.getByPlaceholderText("Body"),
      "Call me back please.",
    );
    await pickPurpose(PURPOSE_LABEL.transactional);
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText(/has not granted consent/i)).toBeTruthy();
    // The rep's words survive the refusal — proving this, not merely that an
    // error rendered, is the whole point: losing a written reply to a 409
    // once is what makes a rep stop trusting the surface.
    expect(screen.getByDisplayValue("Call me back please.")).toBeTruthy();
  });
});

describe("TimelineActions", () => {
  const email: Activity = {
    ...activity202,
    id: "a1",
    kind: "email",
  };
  const note: Activity = {
    ...activity202,
    id: "a2",
    kind: "note",
  };

  it("offers Reply on an email row and Relink alongside it", () => {
    stubRoutes();
    render(
      <TimelineActions activity={email} entityType="deal" entityId="d1" />,
    );
    expect(screen.getByRole("button", { name: "Reply" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  it("offers Reply on a non-email row too, as the send path allows", () => {
    // A send anchored to a note carries no RFC822 identity and starts a
    // conversation, which the backend handles. Gating this on kind === "email"
    // is what made "log a note → compose → send" unreachable in a workspace
    // whose timeline holds nothing captured from mail.
    stubRoutes();
    render(<TimelineActions activity={note} entityType="deal" entityId="d1" />);
    expect(screen.getByRole("button", { name: "Reply" })).toBeTruthy();
    // Relink is always available — the row is already linked to this timeline's
    // entity, and the Activity list payload carries no `links` to gate on.
    expect(screen.getByRole("button", { name: "Relink" })).toBeTruthy();
  });

  it("opens the composer anchored to a note row", async () => {
    stubRoutes();
    render(<TimelineActions activity={note} entityType="deal" entityId="d1" />);
    await userEvent.click(screen.getByRole("button", { name: "Reply" }));
    expect(await screen.findByText("Send this email?")).toBeTruthy();
  });

  it("opens the composer when Reply is clicked", async () => {
    stubRoutes();
    render(
      <TimelineActions activity={email} entityType="deal" entityId="d1" />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Reply" }));
    // The ConfirmModal titled "Send this email?" mounts only once Reply opens it.
    expect(await screen.findByText("Send this email?")).toBeTruthy();
  });

  it("offers no reply when the person is unreachable", async () => {
    // A blocked (or never-established) Telegram identity means a reply box
    // here would only fail once the rep has already written the message —
    // worse than never offering it (design §9.3).
    const telegram: Activity = {
      ...activity202,
      id: "a3",
      kind: "message",
      channel_provider: "telegram",
    };
    stubRoutes({
      "GET /people/p-1": () =>
        jsonResponse({
          id: "p-1",
          full_name: "Jane Doe",
          reachability: [
            {
              provider: "telegram",
              reachable: false,
              since: "2026-07-01T00:00:00Z",
            },
          ],
          version: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        }),
    });
    render(
      <TimelineActions
        activity={telegram}
        entityType="person"
        entityId="p-1"
        personId="p-1"
      />,
    );

    // Relink renders immediately (it never waits on reachability), which is
    // the interaction that proves the component actually mounted and the
    // missing Reply button is the gate, not an unrendered tree.
    expect(await screen.findByRole("button", { name: "Relink" })).toBeTruthy();
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull(),
    );
  });
});

// An account-started send is the SAME send with a different origin (ADR-0087
// §1): no anchor, a fresh thread, and the records it is filed under named
// explicitly. These three cases fix that the composer picks the right door —
// the reply path and the account path are one component, and the thing most
// easily broken by a refactor is which endpoint it reaches for.
describe("ComposeModal started from an account", () => {
  it("sends through POST /emails, filed under the record it started from", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find((r) => r.key === "POST /emails");
    expect(req?.body).toEqual({
      subject: "Hi there",
      body: "Body content",
      to: ["a@x.com"],
      consent_purpose: "transactional",
      // Without a link the message belongs to no record and nobody finds it
      // again, which is the gap this origin exists to close.
      links: [{ entity_type: "organization", entity_id: "org-1" }],
    });
    // ADR-0055 holds on this origin too: the human's click is the approval.
    expect(req?.headers.get("X-Approval-Token")).toBeNull();
  });

  it("never reaches the anchored reply endpoint", async () => {
    const sent = stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox");
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(sent.some((r) => r.key === "POST /emails")).toBe(true),
    );
    // A fabricated anchor is exactly what ADR-0087 forbids, and a send that
    // reached the reply path without one would 404 on a made-up id.
    expect(sent.some((r) => r.key.includes("send-email"))).toBe(false);
  });

  // The grounded account-started draft (ADR-0087/A132) needs a recipient
  // before it can say anything: that is the one thing it knows that an empty
  // compose box does not. The button is disabled with the picker directly
  // above it, rather than running and coming back with a refusal about a
  // field already on screen.
  it("will not draft from an account until a recipient is chosen", async () => {
    const sent = stubRoutes({});
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    const drafting = screen.getByRole("button", { name: "Draft with AI" });
    expect(drafting).toHaveProperty("disabled", true);
    expect(sent.some((r) => r.key.includes("draft-email"))).toBe(false);
    // Undraftable is not unsendable: the rep writes it themselves and sends.
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
  });

  // A draft belongs to the pair it was written for. Showing a message
  // addressed to B that carries A's words, A's disclosure and A's reasons is
  // the mistake nobody catches before pressing Send — and re-drafting could
  // not repair it, because the fill never clobbers a non-empty field.
  it("retires the draft when the recipient changes", async () => {
    stubRoutes({
      "GET /organizations/org-1/360": () =>
        jsonResponse({
          state: "ready",
          as_of: "2026-08-09T09:00:00Z",
          organization: {
            id: "org-1",
            display_name: "Acme",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          people: {
            data: [
              {
                person_id: "p-1",
                full_name: "Sarah Cole",
                strength: {
                  score: 40,
                  bucket: "moderate",
                  factors: {
                    recency: 0,
                    frequency: 0,
                    reciprocity: 0,
                    direction: 0,
                  },
                },
                deal_roles: [],
                consent: {},
              },
              {
                person_id: "p-2",
                full_name: "Mark Hughes",
                strength: {
                  score: 20,
                  bucket: "weak",
                  factors: {
                    recency: 0,
                    frequency: 0,
                    reciprocity: 0,
                    direction: 0,
                  },
                },
                deal_roles: [],
                consent: {},
              },
            ],
            page: { has_more: false, next_cursor: null },
          },
        }),
      "POST /organizations/org-1/draft-email": () =>
        jsonResponse({
          subject: "For Sarah",
          body: "Hi Sarah, shall we pick this up?",
          to: ["sarah@acme.test"],
          generated_by: "model",
          ai_generated: true,
          ai_disclosure: "This message was drafted with AI assistance.",
          reasoning: [{ kind: "recipient", label: "Sarah Cole" }],
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    const user = userEvent.setup();
    await pickOption(
      user,
      await screen.findByRole("combobox", { name: "Draft to" }),
      "Sarah Cole",
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await waitFor(() =>
      expect(screen.getByPlaceholderText("Body")).toHaveProperty(
        "value",
        "Hi Sarah, shall we pick this up?",
      ),
    );
    expect(screen.getByText(/Based on/)).toBeTruthy();

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Draft to" }),
      "Mark Hughes",
    );

    // Sarah's draft, her address, her disclosure and her reasons all go with
    // her. The rep drafts again for Mark, or writes it themselves.
    expect(screen.getByPlaceholderText("Body")).toHaveProperty("value", "");
    expect(screen.queryByText(/Based on/)).toBeNull();
    expect(screen.queryByText(/AI assistance/)).toBeNull();
  });

  // A check-in mail to an account nobody has spoken to in a while is the FIRST
  // message on a delivery: no thread exists, so nothing files it automatically,
  // and it is exactly the message that needs a project chosen. The account
  // having no contact yet is a dead end for the DRAFT — the model has no
  // relationship to write from — and it used to take the project picker with
  // it, so the mail landed unfiled and the ladder asked about it afterwards.
  it("still offers the project when the account has no contact yet", async () => {
    stubRoutes({
      "GET /organizations/org-1/360": () =>
        jsonResponse({
          state: "ready",
          as_of: "2026-08-09T09:00:00Z",
          organization: {
            id: "org-1",
            display_name: "Acme",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          people: { data: [], page: { has_more: false } },
          projects: [
            {
              project_id: "pr-1",
              name: "Zeta migration",
              key: "ZM-1",
              phase: "initiative",
            },
          ],
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(
      await screen.findByText(/No contact on this account yet/),
    ).toBeTruthy();
    // The dead end is about the DRAFT, not about which body of work the
    // message is for.
    expect(
      await screen.findByRole("combobox", { name: /Project/ }),
    ).toBeTruthy();
  });

  // The composer's project control, on a reply. A reply inherits its thread's
  // project on its own (capture's stickiness rung), and the composer suggests
  // that project so the rep does not retype what is already true — but it is a
  // SUGGESTION in a picker they can set to None, not a fact stated at them.
  const replyBackend = (
    links: { entity_type: string; entity_id: string }[],
    projects: { project_id: string; name: string; key: string }[],
  ) =>
    stubRoutes({
      "GET /activities/act-1": () =>
        jsonResponse({
          id: "act-1",
          kind: "email",
          occurred_at: "2026-08-09T09:00:00Z",
          source: "gmail",
          created_at: "2026-08-09T09:00:00Z",
          updated_at: "2026-08-09T09:00:00Z",
          version: 1,
          links,
        }),
      "GET /deals/d-1": () =>
        jsonResponse({
          id: "d-1",
          name: "netcare",
          organization_id: "o-1",
          project_id: "pr-9",
        }),
      "GET /organizations/o-1/360": () =>
        jsonResponse({
          as_of: "2026-08-09T09:00:00Z",
          organization: {
            id: "o-1",
            display_name: "netcare",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          people: { data: [], page: { next_cursor: null } },
          deals: { data: [], page: { next_cursor: null } },
          projects: projects.map((one) => ({
            ...one,
            phase: "delivering",
          })),
        }),
      "GET /projects/pr-9": () =>
        jsonResponse({ id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }),
      "GET /projects/pr-1": () =>
        jsonResponse({ id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" }),
    });

  const openReply = async () => {
    render(
      <ComposeModal
        activityId="act-1"
        entityType="deal"
        entityId="d-1"
        open
        onClose={vi.fn()}
      />,
    );
    return () => screen.getByPlaceholderText("Subject") as HTMLInputElement;
  };

  it("offers the company's projects and defaults to the thread's own", async () => {
    replyBackend(
      [
        { entity_type: "deal", entity_id: "d-1" },
        { entity_type: "project", entity_id: "pr-1" },
      ],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();

    const picker = await screen.findByLabelText("Project");
    // The thread's own project is the default, not the deal's: a conversation
    // already filed settled this.
    await waitFor(() =>
      expect(picker.textContent).toContain("ERP rollout Acme"),
    );
    // Both of the company's projects are on offer, plus None.
    await userEvent.setup().click(picker);
    expect(
      (await screen.findAllByRole("option")).map((o) => o.textContent),
    ).toEqual([
      "No project",
      "ERP-27 · ERP rollout Acme",
      "N2P-1 · Netcare 2 project",
    ]);
    await userEvent.setup().keyboard("{Escape}");
    // And the tag is already in the subject — nothing explains it, because the
    // field shows it.
    await waitFor(() => expect(subject().value).toBe("[ERP-27]"));
  });

  it("defaults to the deal's project when the thread names none", async () => {
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();

    const picker = await screen.findByLabelText("Project");
    await waitFor(() =>
      expect(picker.textContent).toContain("Netcare 2 project"),
    );
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
  });

  it("takes the tag out when the rep picks None, and puts another one in", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [
        { project_id: "pr-1", name: "ERP rollout Acme", key: "ERP-27" },
        { project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" },
      ],
    );
    const subject = await openReply();
    const picker = await screen.findByLabelText("Project");
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
    // `[` opens a key descriptor in user-event's parser, so the tag is typed
    // through fireEvent rather than escaped into unreadability.
    await user.clear(subject());
    fireEvent.change(subject(), {
      target: { value: "[N2P-1] Re: Angebot" },
    });

    await pickOption(user, picker, "No project");
    await waitFor(() => expect(subject().value).toBe("Re: Angebot"));

    // Choosing a different project stamps that one instead.
    await pickOption(user, picker, "ERP-27 · ERP rollout Acme");
    await waitFor(() => expect(subject().value).toBe("[ERP-27] Re: Angebot"));
  });

  it("offers nothing when the company reaches no live project", async () => {
    replyBackend([{ entity_type: "deal", entity_id: "d-1" }], []);
    const subject = await openReply();

    await screen.findByRole("button", { name: "Draft with AI" });
    // A list whose only entry is None asks a question with one answer.
    expect(screen.queryByLabelText("Project")).toBeNull();
    expect(subject().value).toBe("");
  });

  it("keeps the tag when the rep types a subject over it", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    // The ordinary thing a rep does next: type the subject. The field is
    // replaced wholesale, and the tag has to survive that — writing it once
    // when the picker moved is what lost it.
    await user.clear(subject());
    await user.type(subject(), "Re: Kurzer Austausch?");

    await waitFor(() =>
      expect(subject().value).toBe("[N2P-1] Re: Kurzer Austausch?"),
    );
  });

  it("puts the tag back when the subject loses it, while a project is chosen", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    // Deleting the tag out of the text is not how a project is unset — the
    // picker is, and it still names one. Leaving the text without it would put
    // the field and the picker in two different states, and the send would
    // honour the picker.
    await user.clear(subject());
    await user.type(subject(), "Re: ohne Tag");

    await waitFor(() => expect(subject().value).toBe("[N2P-1] Re: ohne Tag"));

    // The way to have no tag is to choose No project.
    await pickOption(user, screen.getByLabelText("Project"), "No project");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));
  });
});

// Opened from a record page, the composer named no conversation: To was empty,
// Subject was empty, and nothing on screen said what was being answered — so a
// reader could press "Draft with AI" without knowing who they were writing to
// or which message they were replying to. Reported from the running product in
// those words.
describe("what the composer says it is answering", () => {
  const latestMail: Activity = {
    id: "act-latest",
    kind: "email",
    subject: "Rechnung GR-2026-0207",
    occurred_at: "2026-08-02T09:00:00Z",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-02T09:00:00Z",
    updated_at: "2026-08-02T09:00:00Z",
  };

  it("names the record's latest message, and drafts against it", async () => {
    const sent = stubRoutes({
      "GET /activities": () =>
        jsonResponse({
          data: [latestMail],
          page: { next_cursor: null, has_more: false },
        }),
      "POST /activities/act-latest/draft-email": () =>
        jsonResponse({
          subject: "Re: Rechnung",
          body: "Hallo",
          to: ["d@x.test"],
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The conversation being continued is on screen BEFORE the reader commits
    // to anything.
    expect(await screen.findByText(/Rechnung GR-2026-0207/)).toBeTruthy();

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    // And the draft answers THAT message: the account-wide draft, which needs
    // a recipient named first, is not what a reader gets from a record page.
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-latest/draft-email"),
      ).toBe(true),
    );
  });

  // An empty account is not a failure, and must not be reported as one: the
  // reader is told this starts a new thread rather than left guessing.
  it("says so when the record has no earlier message", async () => {
    stubRoutes({
      "GET /activities": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText(/starts a new thread/i)).toBeTruthy();
  });

  // Who this is going to, on the line, once it is known. The composer used to
  // make the reader pick the person, and picking them is what made the consent
  // purpose an attestation about a named human. The thread supplies the
  // address now, so the reader has to SEE it before they attest anything.
  it("names the recipient once the draft has filled it", async () => {
    stubRoutes({
      "GET /activities": () =>
        jsonResponse({
          data: [latestMail],
          page: { next_cursor: null, has_more: false },
        }),
      "POST /activities/act-latest/draft-email": () =>
        jsonResponse({
          subject: "Re: Rechnung",
          body: "Hallo",
          to: ["dietmar@valantic.test"],
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    // WHAT is being continued, in the conversation itself rather than in a
    // sentence about it: the pane carries the message the draft is answering.
    await screen.findByText(/Rechnung GR-2026-0207/);
    expect(screen.queryByText(/dietmar@valantic.test/)).toBeNull();

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );

    // And WHO, in the field that will actually carry it. The recipient used to
    // be reported in the answering sentence, which the pane replaced — a
    // sentence naming the subject beside a pane showing it read as a stutter.
    expect(await screen.findByText(/dietmar@valantic.test/)).toBeTruthy();
  });

  // The project narrows through the list's OWN project_id filter. Asking for
  // entity_type=project returns an empty page rather than an error — verified
  // against the running API — so the wrong spelling reads as "no earlier
  // message" for every project on the installation, silently.
  it("narrows to the selected project through project_id", async () => {
    const asked: { params?: URLSearchParams } = {};
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = new URL(
          input instanceof Request ? input.url : String(input),
          "https://test.local",
        );
        if (url.pathname.endsWith("/activities")) {
          asked.params = url.searchParams;
          return jsonResponse({
            data: [latestMail],
            page: { next_cursor: null, has_more: false },
          });
        }
        if (url.pathname.endsWith("/consent-purposes")) {
          return jsonResponse(PURPOSES);
        }
        if (url.pathname.endsWith("/voice-profiles")) {
          return jsonResponse(NO_VOICE_PROFILE);
        }
        return jsonResponse({});
      }),
    );
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByText(/Rechnung GR-2026-0207/);
    // The record still scopes the read; a project only narrows it further.
    expect(asked.params?.get("entity_type")).toBe("organization");
    expect(asked.params?.get("entity_id")).toBe("org-1");
    expect(asked.params?.get("kind")).toBe("email");
    // And no project is selected here, so none is asked for.
    expect(asked.params?.get("project_id")).toBeNull();
  });

  // The draft and the SEND must agree on what is being answered. Split, the
  // draft answers a thread and writes "Re: …" while the send takes the account
  // path: the message files under the links the body names rather than the
  // anchor's own, so the person it was actually with gets none of it, and it
  // leaves as a new RFC chain — an orphan, to a reader who was shown a reply.
  it("sends against the same message it drafted against", async () => {
    const sent = stubRoutes({
      "GET /activities": () =>
        jsonResponse({
          data: [latestMail],
          page: { next_cursor: null, has_more: false },
        }),
      "POST /activities/act-latest/draft-email": () =>
        jsonResponse({
          subject: "Re: Rechnung",
          body: "Hallo",
          to: ["d@x.test"],
        }),
      "POST /activities/act-latest/send-email": () => jsonResponse({}, 202),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByText(/Rechnung GR-2026-0207/);
    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-latest/draft-email"),
      ).toBe(true),
    );

    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => {
      // The reply path, not POST /emails — which would file under the body's
      // own links and start a new thread.
      expect(sent.map((s) => s.key)).toContain(
        "POST /activities/act-latest/send-email",
      );
      expect(sent.map((s) => s.key)).not.toContain("POST /emails");
    });
  });

  // A message the reader may DISCOVER but not read is not an anchor: the list
  // is discover-gated and returns it with its subject and body nulled, while
  // the draft endpoint is content-gated and answers 404. Anchoring there hides
  // the account path — the reader's only working route — and leaves them told
  // they are replying to something the server will not let them answer.
  it("does not anchor on a message whose content is withheld", async () => {
    stubRoutes({
      "GET /activities": () =>
        jsonResponse({
          data: [
            {
              ...latestMail,
              subject: null,
              body: null,
              content_state: "withheld",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(
      <ComposeModal
        entityType="organization"
        entityId="org-1"
        open
        onClose={vi.fn()}
      />,
    );

    // It falls back to the account path, which asks the reader to name a
    // recipient — the honest state, rather than a dead end.
    expect(await screen.findByText(/starts a new thread/i)).toBeTruthy();
  });

  // A caller that named the message has already shown it — the page behind the
  // dialog IS that message — so the line would repeat what the reader opened.
  it("stays quiet when the caller already named the message", async () => {
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(activity202),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="person"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByRole("combobox");
    expect(screen.queryByText(/Replying to|starts a new thread/i)).toBeNull();
  });
});

// The conversation a reply is answering, in the drawer beside it.
//
// The composer used to keep the centred box for a reply on the grounds that the
// message was "on the page behind" — which the box was covering. These pin the
// shape it takes and what it draws in the second column.
describe("the composer's conversation pane", () => {
  const threaded: Activity = {
    ...activity202,
    thread_key: "<t-1@mail>",
    body: "The signed order is attached.",
    direction: "inbound",
  };
  const sibling: Activity = {
    ...threaded,
    id: "act-0",
    subject: "Q3 order",
    body: "Sending the order over today.",
    direction: "outbound",
    occurred_at: "2026-06-28T00:00:00Z",
  };

  it("takes the drawer on an account whose mail it is answering", async () => {
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(threaded),
      "GET /activities": () =>
        jsonResponse({ data: [threaded, sibling], page: { has_more: false } }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="organization"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The sibling message arrives in the pane, which is what widens the
    // drawer — so wait for the conversation, then read the shape.
    expect(
      await screen.findByText("Sending the order over today."),
    ).toBeTruthy();
    // The drawer, not the centred box: an account's mail keeps the record
    // beside it whether it opens a conversation or answers one.
    expect(document.querySelector(".modal-drawer")).not.toBeNull();
    expect(document.querySelector(".modal-drawer-split")).not.toBeNull();
  });

  it("draws a message it may not read as a held place, not a gap", async () => {
    // A thread can run through an audience this reader is not in. Dropping the
    // row would make the conversation look continuous when it is not, and the
    // reply would be written into a silence the writer cannot see.
    const withheld: Activity = {
      ...sibling,
      id: "act-2",
      subject: null,
      body: null,
      content_state: "withheld",
    };
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(threaded),
      "GET /activities": () =>
        jsonResponse({ data: [threaded, withheld], page: { has_more: false } }),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="organization"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    const pane = await screen.findByRole("region", {
      name: /this conversation/i,
    });
    expect(pane.querySelectorAll("li")).toHaveLength(2);
  });

  it("shows a mail with no thread as the one message it is", async () => {
    // Capture assigns a thread key only where a provider reported a
    // conversation. A mail without one is a conversation of exactly one
    // message, and asking the server for every activity that has no thread is
    // the wrong question.
    const loose: Activity = { ...threaded, thread_key: null };
    const sent = stubRoutes({
      "GET /activities/act-1": () => jsonResponse(loose),
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="organization"
        entityId="o-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(
      await screen.findByText("The signed order is attached."),
    ).toBeTruthy();
    expect(sent.filter((call) => call.key === "GET /activities")).toHaveLength(
      0,
    );
  });
});
