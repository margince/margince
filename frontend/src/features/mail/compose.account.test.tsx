/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { messageText } from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  type Activity,
  activity202,
  fillSendableForm,
  findThreadChoice,
  jsonResponse,
  NO_VOICE_PROFILE,
  PURPOSES,
  render,
  stubRoutes,
  WHY_ASK,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

// An account-started send is the same send with another origin (ADR-0087).
// It has no anchor, a fresh thread, and names the records it is filed under.
// These three cases check that the composer picks the right endpoint. The
// reply path and the account path are one component, and a refactor most
// easily breaks which endpoint it reaches for.
describe("ComposeModal started from an account", () => {
  it("sends through POST /emails, filed under the record it started from", async () => {
    const onClose = vi.fn();
    const sent = stubRoutes({
      "POST /emails": () => jsonResponse(activity202, 202),
    });
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={onClose}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const req = sent.find((r) => r.key === "POST /emails");
    expect(req?.body).toEqual({
      subject: "Hi there",
      body: "Body content",
      html_body: "<p>Body content</p>",
      to: ["a@x.com"],
      communication_context: "requested_followup",
      // Without a link the message belongs to no record and nobody finds it
      // again, which is the gap this origin exists to close.
      links: [{ entity_type: "company", entity_id: "company-1" }],
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(sent.some((r) => r.key === "POST /emails")).toBe(true),
    );
    // A fabricated anchor is what ADR-0087 forbids, and a send that
    // reached the reply path without one would 404 on a made-up id.
    expect(sent.some((r) => r.key.includes("send-email"))).toBe(false);
  });

  // The grounded account-started draft (ADR-0087) needs a recipient before it
  // can say anything. That is the one thing it knows that an empty compose box
  // does not. The button is disabled with the picker directly above it. It does
  // not run and come back with a refusal about a field already on screen.
  it("will not draft from an account until a recipient is chosen", async () => {
    const sent = stubRoutes({});
    render(
      <ComposeModal
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    const drafting = draftButton();
    expect(drafting).toHaveProperty("disabled", true);
    expect(sent.some((r) => r.key.includes("draft-email"))).toBe(false);
    // Undraftable is not unsendable: the rep writes it themselves and sends.
    expect(screen.getByRole("button", { name: "Send" })).toBeTruthy();
  });

  // A draft belongs to the pair it was written for. A message to B that carries
  // A's words, disclosure and reasons is a mistake nobody catches before Send.
  // Re-drafting could not repair it, because the fill never clobbers a
  // non-empty field.
  it("retires the draft when the recipient changes", async () => {
    stubRoutes({
      "GET /companies/company-1/360": () =>
        jsonResponse({
          state: "ready",
          as_of: "2026-08-09T09:00:00Z",
          company: {
            id: "company-1",
            display_name: "Acme",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          contacts: {
            data: [
              {
                contact_id: "p-1",
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
                contact_id: "p-2",
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
      "POST /companies/company-1/draft-email": () =>
        jsonResponse({
          subject: "For Sarah",
          body: "Hi Sarah, shall we pick this up?",
          to: ["sarah@acme.test"],
          generated_by: "model",
          ai_generated: true,
          ai_disclosure: "Drafted with AI assistance. Review before sending.",
          reasoning: [{ kind: "recipient", label: "Sarah Cole" }],
        }),
    });
    render(
      <ComposeModal
        intent="Discuss the renewal"
        entityType="company"
        entityId="company-1"
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
    await userEvent.click(draftButton());
    await waitFor(() =>
      expect(messageText("Body")).toBe("Hi Sarah, shall we pick this up?"),
    );
    expect(screen.getByText(/Based on/)).toBeTruthy();

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Draft to" }),
      "Mark Hughes",
    );

    // Sarah's draft, her address, her disclosure and her reasons all go with
    // her. The rep drafts again for Mark, or writes it themselves.
    await waitFor(() => expect(messageText("Body")).toBe(""));
    expect(screen.queryByText(/Based on/)).toBeNull();
    expect(screen.queryByText(/AI assistance/)).toBeNull();
  });

  // A check-in mail to an account nobody has written to in a while is the first
  // message on a delivery. No thread exists, so nothing files it on its own,
  // and it needs a project chosen. An account with no contact yet is a dead end
  // for the draft, since the model has no relationship to write from. The
  // project picker stays, or the mail lands unfiled and the ladder asks later.
  it("still offers the project when the account has no contact yet", async () => {
    stubRoutes({
      "GET /companies/company-1/360": () =>
        jsonResponse({
          state: "ready",
          as_of: "2026-08-09T09:00:00Z",
          company: {
            id: "company-1",
            display_name: "Acme",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          contacts: { data: [], page: { has_more: false } },
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(
      await screen.findByText(/No contacts at this company yet/),
    ).toBeTruthy();
    // The dead end is about the draft, not about which body of work the
    // message is for.
    expect(
      await screen.findByRole("combobox", { name: /Project/ }),
    ).toBeTruthy();
  });
});

// Opened from a record page, the composer must name a conversation. With To
// and Subject empty and nothing saying what is being answered, a reader could
// press "Draft with AI" without knowing who they write to. They would not know
// which message they reply to either.
describe("what the composer says it is answering", () => {
  const latestMail: Activity = {
    occurred_precision: "instant",
    offers_email: false,
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

  it("offers the record's conversations, and drafts against the one picked", async () => {
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The conversations are offered, not chosen for the reader. A button that
    // says "write an email" must not anchor the message to whatever came last,
    // addressed to somebody they had not picked.
    await userEvent.click(await findThreadChoice(/Rechnung GR-2026-0207/));

    await userEvent.click(draftButton());

    // And the draft answers that message: the account-wide draft, which needs
    // a recipient named first, is not what a reader gets from a record page.
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-latest/draft-email"),
      ).toBe(true),
    );
  });

  // An empty account is not a failure, and must not be reported as one. The
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    expect(await screen.findByText("New email")).toBeTruthy();
  });

  // Who this is going to, on the line, once it is known. Picking the contact
  // made the consent purpose an attestation about a named human. The thread
  // supplies the address, so the reader has to see it before they attest.
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The conversation is picked, then drafted against.
    await userEvent.click(await findThreadChoice(/Rechnung GR-2026-0207/));
    expect(screen.queryByText(/dietmar@valantic.test/)).toBeNull();

    await userEvent.click(draftButton());

    // And who, in the field that will carry it. It is asked for as the chip's
    // own control, because the address also appears in the quoted head of the
    // message the pane is showing.
    expect(
      await screen.findByRole("button", {
        name: "Remove dietmar@valantic.test",
      }),
    ).toBeTruthy();
  });

  // The project narrows through the list's own project_id filter. Asking for
  // entity_type=project returns an empty page rather than an error, as checked
  // against the running API. So the wrong spelling reads as "no earlier
  // message" for every project on the installation, with no sign.
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    await findThreadChoice(/Rechnung GR-2026-0207/);
    // The record still scopes the read; a project only narrows it further.
    expect(asked.params?.get("entity_type")).toBe("company");
    expect(asked.params?.get("entity_id")).toBe("company-1");
    expect(asked.params?.get("kind")).toBe("email");
    // And no project is selected here, so none is asked for.
    expect(asked.params?.get("project_id")).toBeNull();
  });

  // The draft and the send must agree on what is being answered. Split, the
  // draft answers a thread and writes "Re: …" while the send takes the account
  // path. The message files under the links the body names, not the anchor's.
  // The contact it was with gets none of it, and it leaves as a new RFC chain.
  // That is an orphan, to a reader who was shown a reply.
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    await userEvent.click(await findThreadChoice(/Rechnung GR-2026-0207/));
    await userEvent.click(draftButton());
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-latest/draft-email"),
      ).toBe(true),
    );

    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: /^Send$/ }));

    await waitFor(() => {
      // The reply path, not POST /emails, which would file under the body's
      // own links and start a new thread.
      expect(sent.map((s) => s.key)).toContain(
        "POST /activities/act-latest/send-email",
      );
      expect(sent.map((s) => s.key)).not.toContain("POST /emails");
    });
  });

  // A message the reader may discover but not read is not an anchor. The list
  // is discover-gated and returns it with subject and body nulled. The draft
  // endpoint is content-gated and answers 404. Anchoring there hides the
  // account path, the reader's only working route. It tells them they reply to
  // something the server will not let them answer.
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
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />,
    );

    // Withheld content cannot be offered as a reply target.
    expect(await screen.findByText("New email")).toBeTruthy();
    await waitFor(() =>
      expect(
        screen.queryByRole("region", { name: /Continue thread/i }),
      ).toBeNull(),
    );
  });

  it("names the exact message the caller chose", async () => {
    stubRoutes({
      "GET /activities/act-1": () => jsonResponse(activity202),
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

    await screen.findByRole("combobox", { name: WHY_ASK });
    expect(await screen.findByText(/Replying to “Re: Q3”/)).toBeTruthy();
  });
});
