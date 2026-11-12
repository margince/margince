// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  messageBox,
  messageText,
  writeMessage,
} from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { draftButton } from "../../testing/composedraft.testkit";
import { ComposeModal } from "./compose";
import {
  jsonResponse,
  problemResponse,
  render,
  type Sent,
  stubRoutes,
} from "./compose.testkit";

// The rep picks the language of an AI draft. The server keeps no choice
// between calls, so these read what each request names.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const INTENT = "Confirm the delivery window";
const KEPT_NOTICE = "Your edits were kept. Draft again when ready.";
const CONTACT_ROUTE = "POST /contacts/p-1/draft-email";

function drafted(body: string, language?: string) {
  return {
    subject: "Delivery window",
    body,
    to: ["dana@example.test"],
    ai_generated: true,
    ai_disclosure: "Drafted with AI assistance. Review before sending.",
    ...(language ? { language } : {}),
  };
}

// Serves the drafts in order, the last one again for any later call.
function draftsInTurn(...answers: (() => Response | Promise<Response>)[]) {
  let call = 0;
  return () => {
    const answer = answers[Math.min(call, answers.length - 1)];
    call += 1;
    return answer ? answer() : jsonResponse({});
  };
}

const answer = (body: string, language?: string) => () =>
  jsonResponse(drafted(body, language));

function contactComposer(): ReactNode {
  return (
    <ComposeModal
      intent={INTENT}
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      open
      onClose={vi.fn()}
    />
  );
}

function languagePicker() {
  return screen.getByRole("combobox", { name: "Draft language" });
}

function draftCalls(sent: Sent[], route: string) {
  return sent.filter((call) => call.key === route);
}

async function firstDraft(user: UserEvent, body: string) {
  await waitFor(() => expect(draftButton()).toHaveProperty("disabled", false));
  await user.click(draftButton());
  await waitFor(() => expect(messageText("Body")).toBe(body));
}

const COMPANY_360 = {
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
        contact_id: "c-9",
        attachable: true,
        full_name: "Dana Roth",
        primary_email: "dana@example.test",
        strength: {
          score: 20,
          bucket: "weak",
          factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
        },
        deal_roles: [],
        consent: {},
      },
    ],
    page: { has_more: false, next_cursor: null },
  },
};

// Every drafting origin the composer serves, each with its own route.
const ORIGINS = [
  {
    origin: "a contact",
    route: CONTACT_ROUTE,
    reads: {},
    ui: contactComposer,
    before: async () => {},
  },
  {
    origin: "a lead",
    route: "POST /leads/l-1/draft-email",
    reads: {},
    ui: () => (
      <ComposeModal
        intent={INTENT}
        entityType="lead"
        entityId="l-1"
        recordAddress="dana@example.test"
        open
        onClose={vi.fn()}
      />
    ),
    before: async () => {},
  },
  {
    origin: "a company",
    route: "POST /companies/company-1/draft-email",
    reads: { "GET /companies/company-1/360": () => jsonResponse(COMPANY_360) },
    ui: () => (
      <ComposeModal
        intent={INTENT}
        entityType="company"
        entityId="company-1"
        open
        onClose={vi.fn()}
      />
    ),
    before: async (user: UserEvent) => {
      await user.type(await screen.findByLabelText("To"), "dana@example.test");
      await user.keyboard("{Enter}");
    },
  },
  {
    origin: "a reply",
    route: "POST /activities/act-1/draft-email",
    reads: {},
    ui: () => (
      <ComposeModal
        intent={INTENT}
        activityId="act-1"
        entityType="contact"
        entityId="p-1"
        open
        onClose={vi.fn()}
      />
    ),
    before: async () => {},
  },
] as const;

describe("the language of an AI draft", () => {
  it("is named on the picker as the draft reported it", async () => {
    const user = userEvent.setup();
    stubRoutes({ [CONTACT_ROUTE]: answer("Hallo Dana,", "de") });
    render(contactComposer());

    await firstDraft(user, "Hallo Dana,");

    expect(languagePicker().textContent).toContain("Deutsch");
  });

  it.each(ORIGINS)(
    "writes the words on screen again in the picked language, from $origin",
    async ({ route, reads, ui, before }) => {
      const user = userEvent.setup();
      const sent = stubRoutes({
        ...reads,
        [route]: draftsInTurn(
          answer("Hello Dana,", "en"),
          answer("Chào chị Dana,", "vi"),
        ),
      });
      render(ui());
      await before(user);
      await firstDraft(user, "Hello Dana,");

      await pickOption(user, languagePicker(), "Tiếng Việt");

      await waitFor(() => expect(messageText("Body")).toBe("Chào chị Dana,"));
      expect(draftCalls(sent, route)[1]?.body).toMatchObject({
        intent: INTENT,
        rewrite_of: "Hello Dana,",
        language: "vi",
      });
      expect(languagePicker().textContent).toContain("Tiếng Việt");
    },
  );

  it("stays on offer over words the rep edited, and revises those words", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer("Hallo Dana, mit meiner Zeile.", "de"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");

    writeMessage("Body", "Hello Dana, and my own line.");

    expect(screen.getByRole("button", { name: "Shorter" })).toBeTruthy();
    await pickOption(user, languagePicker(), "Deutsch");
    await waitFor(() =>
      expect(messageText("Body")).toBe("Hallo Dana, mit meiner Zeile."),
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[1]?.body).toMatchObject({
      rewrite_of: "Hello Dana, and my own line.",
      language: "de",
    });
  });

  it("replaces the words with a draft no model wrote, in the picked language", async () => {
    const user = userEvent.setup();
    const unmodelled = (body: string, language: string) => () =>
      jsonResponse({ ...drafted(body, language), ai_generated: false });
    stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        unmodelled("Hello Dana,", "en"),
        unmodelled("Hallo Dana,", "de"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");

    await pickOption(user, languagePicker(), "Deutsch");

    await waitFor(() => expect(messageText("Body")).toBe("Hallo Dana,"));
    expect(languagePicker().textContent).toContain("Deutsch");
  });

  it("is named again by every later ask once the rep picked it", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer("Chào chị Dana,", "vi"),
        answer("Chào chị,", "vi"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");
    await pickOption(user, languagePicker(), "Tiếng Việt");
    await waitFor(() => expect(messageText("Body")).toBe("Chào chị Dana,"));

    await user.click(screen.getByRole("button", { name: "Shorter" }));
    await waitFor(() => expect(messageText("Body")).toBe("Chào chị,"));
    await user.click(screen.getByRole("button", { name: "Change the ask" }));
    await user.click(draftButton());
    await waitFor(() =>
      expect(draftCalls(sent, CONTACT_ROUTE)).toHaveLength(4),
    );

    const [, , shorter, again] = draftCalls(sent, CONTACT_ROUTE);
    expect(shorter?.body).toMatchObject({
      intent: "Keep the meaning and use fewer words.",
      language: "vi",
    });
    expect(again?.body).toMatchObject({ intent: INTENT, language: "vi" });
  });

  it("is left to the server while the rep has picked none", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hallo Dana,", "de"),
        answer("Hallo,", "de"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hallo Dana,");

    await user.click(screen.getByRole("button", { name: "Shorter" }));

    await waitFor(() => expect(messageText("Body")).toBe("Hallo,"));
    for (const call of draftCalls(sent, CONTACT_ROUTE)) {
      expect(call.body).not.toHaveProperty("language");
    }
  });

  it("offers no picker when the draft reports no language", async () => {
    const user = userEvent.setup();
    stubRoutes({ [CONTACT_ROUTE]: answer("Hello Dana,") });
    render(contactComposer());

    await firstDraft(user, "Hello Dana,");

    expect(screen.queryByRole("combobox", { name: "Draft language" })).toBe(
      null,
    );
  });

  it("is held while its draft is out", async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => {};
    stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        () =>
          new Promise<Response>((resolve) => {
            release = resolve;
          }),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");

    await pickOption(user, languagePicker(), "Deutsch");

    await waitFor(() =>
      expect(languagePicker()).toHaveProperty("disabled", true),
    );
    release(jsonResponse(drafted("Hallo Dana,", "de")));
    await waitFor(() => expect(messageText("Body")).toBe("Hallo Dana,"));
    expect(languagePicker()).toHaveProperty("disabled", false);
  });

  it("keeps words the rep edited when no model writes them again", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        () =>
          jsonResponse({
            ...drafted("Hallo Dana,", "de"),
            ai_generated: false,
          }),
        answer("Hello again,", "en"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");
    writeMessage("Body", "Hello Dana, and my own line.");

    await pickOption(user, languagePicker(), "Deutsch");

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageText("Body")).toBe("Hello Dana, and my own line.");
    expect(languagePicker().textContent).toContain("English");
    await user.click(screen.getByRole("button", { name: "Change the ask" }));
    await user.click(draftButton());
    await waitFor(() =>
      expect(draftCalls(sent, CONTACT_ROUTE)).toHaveLength(3),
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[2]?.body).not.toHaveProperty(
      "language",
    );
  });

  it("keeps the rep's own link, which goes out with nothing to revise", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer("Hallo Dana,", "de"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");
    writeMessage(
      "Body",
      'Hello Dana, <a href="https://plan.test">the plan</a>',
    );

    await pickOption(user, languagePicker(), "Deutsch");

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageBox("Body").innerHTML).toContain("https://plan.test");
    expect(draftCalls(sent, CONTACT_ROUTE)[1]?.body).not.toHaveProperty(
      "rewrite_of",
    );
  });

  it("keeps the rep's own bold on the served words, which go out with nothing to revise", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer("Hallo Dana,", "de"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");
    writeMessage("Body", "<strong>Hello Dana,</strong>");

    await pickOption(user, languagePicker(), "Deutsch");

    expect(await screen.findByText(KEPT_NOTICE)).toBeTruthy();
    expect(messageBox("Body").innerHTML).toContain(
      "<strong>Hello Dana,</strong>",
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[1]?.body).not.toHaveProperty(
      "rewrite_of",
    );
  });

  it("goes back with the words an undo brings back", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer("Chào chị Dana,", "vi"),
        answer("Hello again,", "en"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");
    await pickOption(user, languagePicker(), "Tiếng Việt");
    await waitFor(() => expect(messageText("Body")).toBe("Chào chị Dana,"));

    writeMessage("Body", "Hello Dana,");

    await waitFor(() =>
      expect(languagePicker().textContent).toContain("English"),
    );
    await user.click(screen.getByRole("button", { name: "Change the ask" }));
    await user.click(draftButton());
    await waitFor(() =>
      expect(draftCalls(sent, CONTACT_ROUTE)).toHaveLength(3),
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[2]?.body).not.toHaveProperty(
      "language",
    );
  });

  it("offers no picker and names nothing for a language this build does not ship", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Bonjour Dana,", "fr"),
        answer("Bonjour,", "fr"),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Bonjour Dana,");

    expect(screen.queryByRole("combobox", { name: "Draft language" })).toBe(
      null,
    );
    await user.click(screen.getByRole("button", { name: "Shorter" }));
    await waitFor(() => expect(messageText("Body")).toBe("Bonjour,"));
    for (const call of draftCalls(sent, CONTACT_ROUTE)) {
      expect(call.body).not.toHaveProperty("language");
    }
  });

  it("keeps the words and the language when the new draft fails", async () => {
    const user = userEvent.setup();
    stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(answer("Hello Dana,", "en"), () =>
        problemResponse(
          { code: "internal", title: "Server Error", detail: "no model" },
          500,
        ),
      ),
    });
    render(contactComposer());
    await firstDraft(user, "Hello Dana,");

    await pickOption(user, languagePicker(), "Deutsch");

    const announced = await screen.findAllByRole("alert");
    expect(announced.map((node) => node.textContent)).toContain("no model");
    expect(messageText("Body")).toBe("Hello Dana,");
    expect(languagePicker().textContent).toContain("English");
  });
});

describe("the picked language in a saved draft", () => {
  const BODY = "Hallo Dana, passt Dienstag?";
  const SAVED = {
    id: "md-1",
    anchor_type: "contact",
    anchor_id: "p-1",
    to: ["dana@example.test"],
    cc: [],
    bcc: [],
    subject: "Lieferfenster",
    body: BODY,
    version: 1,
    agent_drafted: false,
    created_at: "2026-09-20T09:00:00Z",
    updated_at: "2026-09-20T09:00:00Z",
  };

  it("is saved with the editor state", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const sent = stubRoutes({
      [CONTACT_ROUTE]: draftsInTurn(
        answer("Hello Dana,", "en"),
        answer(BODY, "de"),
      ),
      "PUT /mail-drafts": () => jsonResponse(SAVED),
    });
    render(
      <ComposeModal
        intent={INTENT}
        entityType="contact"
        entityId="p-1"
        contactId="p-1"
        open
        onClose={onClose}
      />,
    );
    await firstDraft(user, "Hello Dana,");
    await pickOption(user, languagePicker(), "Deutsch");
    await waitFor(() => expect(messageText("Body")).toBe(BODY));

    await user.click(screen.getByRole("button", { name: "Close" }));

    await waitFor(() => expect(onClose).toHaveBeenCalled());
    const saves = draftCalls(sent, "PUT /mail-drafts");
    expect(saves.at(-1)?.body).toMatchObject({
      editor_state: { intent: INTENT, language: "de" },
    });
  });

  it("is put back on reload and named by the next rewrite", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /mail-drafts": () =>
        jsonResponse({
          ...SAVED,
          editor_state: {
            intent: INTENT,
            ai_generated: true,
            ai_disclosure: "Mit KI-Unterstützung verfasst.",
            served_body: BODY,
            language: "de",
          },
        }),
      [CONTACT_ROUTE]: answer("Hallo Dana, Dienstag?", "de"),
    });
    render(contactComposer());
    await screen.findByText("Saved draft restored");

    expect(languagePicker().textContent).toContain("Deutsch");
    await user.click(screen.getByRole("button", { name: "Shorter" }));

    await waitFor(() =>
      expect(messageText("Body")).toBe("Hallo Dana, Dienstag?"),
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[0]?.body).toMatchObject({
      rewrite_of: BODY,
      language: "de",
    });
  });

  it("names nothing from a saved choice this build does not ship", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes({
      "GET /mail-drafts": () =>
        jsonResponse({
          ...SAVED,
          editor_state: {
            intent: INTENT,
            ai_generated: true,
            served_body: BODY,
            language: "fr",
          },
        }),
      [CONTACT_ROUTE]: answer("Hallo Dana, Dienstag?", "de"),
    });
    render(contactComposer());
    await screen.findByText("Saved draft restored");

    expect(screen.queryByRole("combobox", { name: "Draft language" })).toBe(
      null,
    );
    await user.click(screen.getByRole("button", { name: "Shorter" }));
    await waitFor(() =>
      expect(messageText("Body")).toBe("Hallo Dana, Dienstag?"),
    );
    expect(draftCalls(sent, CONTACT_ROUTE)[0]?.body).not.toHaveProperty(
      "language",
    );
  });
});
