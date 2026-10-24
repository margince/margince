/** @vitest-environment happy-dom */
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
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
  pickWhy,
  render,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
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

  // The composer's project control, on a reply. A reply inherits its thread's
  // project on its own (capture's stickiness rung). The composer suggests that
  // project so the rep does not retype it. It is a suggestion in a picker they
  // can set to None, not a fact stated at them.
  const replyBackend = (
    links: { entity_type: string; entity_id: string }[],
    projects: { project_id: string; name: string; key: string }[],
    routes: Record<string, () => Response> = {},
  ) =>
    stubRoutes({
      ...routes,
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
          company_id: "o-1",
          project_id: "pr-9",
        }),
      "GET /companies/o-1/360": () =>
        jsonResponse({
          as_of: "2026-08-09T09:00:00Z",
          company: {
            id: "o-1",
            display_name: "netcare",
            source: "manual",
            captured_by: "human:u1",
            created_at: "2026-08-01T00:00:00Z",
            updated_at: "2026-08-01T00:00:00Z",
          },
          sections_omitted: [],
          contacts: { data: [], page: { next_cursor: null } },
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
    return () => screen.getByLabelText("Subject") as HTMLInputElement;
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
    // And the tag is already in the subject. Nothing explains it, because the
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

    await draftButton();
    // A list whose only entry is None asks a question with one answer.
    expect(screen.getByLabelText("Project").hasAttribute("disabled")).toBe(
      true,
    );
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

    // The ordinary thing a rep does next is type the subject. The field is
    // replaced wholesale, and the tag has to survive that. Writing it only when
    // the picker moves would lose it.
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

    // Deleting the tag out of the text does not unset a project. The picker
    // does, and it still names one. Leaving the text without it would put
    // the field and the picker in two different states, and the send would
    // honour the picker.
    await user.clear(subject());
    await user.type(subject(), "Re: ohne Tag");

    await waitFor(() => expect(subject().value).toBe("[N2P-1] Re: ohne Tag"));

    // The way to have no tag is to choose No project.
    await pickOption(user, screen.getByLabelText("Project"), "No project");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));
  });

  // The key reaches the client's inbox, so whether it goes is a switch the rep
  // sees beside the subject. It is on by default, because it files the answer.
  const KEY_SWITCH = "Add project key to subject (helps filing)";
  const savedReply = (subject: string) => ({
    id: "md-1",
    anchor_type: "activity",
    anchor_id: "act-1",
    to: ["kunde@netcare.test"],
    cc: [],
    bcc: [],
    subject,
    body: "Hallo",
    html_body: "<p>Hallo</p>",
    version: 1,
    agent_drafted: false,
    created_at: "2026-08-09T09:00:00Z",
    updated_at: "2026-08-09T09:05:00Z",
  });

  it("shows the key switch on while a project is chosen, and none without one", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();

    const key = await screen.findByRole("switch", { name: KEY_SWITCH });
    expect(key.getAttribute("aria-checked")).toBe("true");
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    await pickOption(user, screen.getByLabelText("Project"), "No project");
    await waitFor(() =>
      expect(screen.queryByRole("switch", { name: KEY_SWITCH })).toBeNull(),
    );
  });

  it("keeps the tag out once turned off, through a draft and a retype", async () => {
    const user = userEvent.setup();
    const sent = replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
      {
        "POST /activities/act-1/draft-email": () =>
          jsonResponse({ subject: "[N2P-1] Re: Angebot", body: "Hallo" }),
        "PUT /mail-drafts": () => jsonResponse(savedReply("Re: ohne Tag"), 200),
        "POST /activities/act-1/send-email": () => jsonResponse({}, 202),
      },
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));

    await user.click(await screen.findByRole("switch", { name: KEY_SWITCH }));
    await waitFor(() => expect(subject().value).toBe(""));

    await user.click(draftButton());
    await waitFor(() =>
      expect(
        sent.some((s) => s.key === "POST /activities/act-1/draft-email"),
      ).toBe(true),
    );
    await waitFor(() => expect(subject().value).toBe("Re: Angebot"));

    await user.clear(subject());
    await user.type(subject(), "Re: ohne Tag");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));
    expect(
      screen
        .getByRole("switch", { name: KEY_SWITCH })
        .getAttribute("aria-checked"),
    ).toBe("false");

    await user.type(screen.getByLabelText("To"), "kunde@netcare.test");
    await user.tab();
    await pickWhy(WHY_LABEL.requestedFollowup);
    await user.click(screen.getByRole("button", { name: /^Send$/ }));
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "POST /activities/act-1/send-email")?.body,
      ).toMatchObject({ subject: "Re: ohne Tag" }),
    );
  });

  // A draft saved with the key off comes back off, and the key stays out. One
  // saved with the key comes back with the switch on.
  it.each([
    ["Re: Angebot", "false"],
    ["[N2P-1] Re: Angebot", "true"],
  ])(
    "takes the switch from a restored draft's subject %s",
    async (saved, on) => {
      replyBackend(
        [{ entity_type: "deal", entity_id: "d-1" }],
        [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
        { "GET /mail-drafts": () => jsonResponse(savedReply(saved)) },
      );
      const subject = await openReply();
      await screen.findByText("Saved draft restored");

      await waitFor(() =>
        expect(
          screen
            .getByRole("switch", { name: KEY_SWITCH })
            .getAttribute("aria-checked"),
        ).toBe(on),
      );
      expect(subject().value).toBe(saved);
    },
  );

  it("puts the tag back when the switch is turned on again", async () => {
    const user = userEvent.setup();
    replyBackend(
      [{ entity_type: "deal", entity_id: "d-1" }],
      [{ project_id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }],
    );
    const subject = await openReply();
    await waitFor(() => expect(subject().value).toBe("[N2P-1]"));
    await user.click(await screen.findByRole("switch", { name: KEY_SWITCH }));
    await user.type(subject(), "Re: ohne Tag");
    await waitFor(() => expect(subject().value).toBe("Re: ohne Tag"));

    // Turned on again, the tag comes back in front of what the rep wrote.
    await user.click(screen.getByRole("switch", { name: KEY_SWITCH }));
    await waitFor(() => expect(subject().value).toBe("[N2P-1] Re: ohne Tag"));
  });
});

// Opened from a record page, the composer must name a conversation. With To
// and Subject empty and nothing saying what is being answered, a reader could
// press "Draft with AI" without knowing who they write to. They would not know
// which message they reply to either.
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
