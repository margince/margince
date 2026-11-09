// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { ComposeModal } from "./compose";
import { jsonResponse, pickWhy, render, stubRoutes } from "./compose.testkit";
import type { EditSeed } from "./schedulededit";

// Which deal a message to a contact is about.
//
// An Active deal reason is proved by a deal. A contact's composer offers the
// contact's open deals beside it, and files the message under the one the rep
// chose. The preview and the send carry the same links, because the
// engine reads the deal from them. These tests assert the request bodies.

const DEAL_PICK = "Deal this follow-up is about";

// One open deal seated twice, one lost deal, and an open deal whose seat ended.
function seat(id: string, deal: string, title: string, extra: object = {}) {
  return {
    relationship_id: id,
    deal_id: deal,
    deal_title: title,
    deal_status: "open",
    seat_current: true,
    role: "champion",
    ...extra,
  };
}

// A first page with no deal to offer, while more seats remain unread: the
// deal the rep means may be among them.
function contactView(hasMore = false) {
  const seats = [
    seat("r-1", "deal-open", "Acme Renewal"),
    seat("r-2", "deal-open", "Acme Renewal", { role: "economic_buyer" }),
    seat("r-3", "deal-lost", "Acme Pilot", { deal_status: "lost" }),
    seat("r-4", "deal-left", "Acme Rollout", { seat_current: false }),
  ];
  return {
    as_of: "2026-09-01T09:00:00Z",
    sections_omitted: [],
    contact: { id: "c-1", full_name: "Anna Weber" },
    deal_roles: {
      data: hasMore ? seats.slice(2) : seats,
      page: { has_more: hasMore },
    },
  };
}

type Sent = ReturnType<typeof stubRoutes>;

function linksOf(sent: Sent, key: string) {
  const bodies = sent.filter((s) => s.key === key).map((s) => s.body);
  return (bodies.at(-1) as { links?: unknown } | undefined)?.links;
}

function openContactComposer(
  edit?: EditSeed,
  view: () => Response | Promise<Response> = () => jsonResponse(contactView()),
) {
  const sent = stubRoutes({
    "GET /contacts/c-1/360": view,
    "POST /emails": () => jsonResponse({ id: "sent-1" }, 202),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      edit={edit}
      open
      onClose={() => {}}
    />,
  );
  return sent;
}

async function writeToAnna() {
  await userEvent.type(screen.getByLabelText("To"), "anna@acme.test");
  await userEvent.tab();
  await userEvent.type(screen.getByLabelText("Subject"), "Next steps");
  writeMessage("Body", "As agreed, the next steps.");
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the deal a message to a contact is about", () => {
  it("offers the open deals once Active deal is the reason, and files under the one chosen", async () => {
    const sent = openContactComposer();
    await writeToAnna();
    expect(screen.queryByRole("combobox", { name: DEAL_PICK })).toBeNull();

    await pickWhy("Active deal");
    const pick = await screen.findByRole("combobox", { name: DEAL_PICK });
    await userEvent.click(pick);
    expect(
      screen.getAllByRole("option", { name: "Deal: Acme Renewal" }),
    ).toHaveLength(1);
    expect(
      screen.queryByRole("option", { name: "Deal: Acme Pilot" }),
    ).toBeNull();
    expect(
      screen.queryByRole("option", { name: "Deal: Acme Rollout" }),
    ).toBeNull();
    await userEvent.keyboard("{Escape}");

    await pickOption(userEvent.setup(), pick, "Deal: Acme Renewal");
    const filed = [
      { entity_type: "contact", entity_id: "c-1" },
      { entity_type: "deal", entity_id: "deal-open" },
    ];
    await waitFor(() =>
      expect(linksOf(sent, "POST /emails:preview")).toEqual(filed),
    );

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await userEvent.click(await screen.findByRole("button", { name: "Send" }));
    await waitFor(() => expect(linksOf(sent, "POST /emails")).toEqual(filed));
  });

  it("holds Send until a deal is chosen, and sends nothing meanwhile", async () => {
    const sent = openContactComposer();
    await writeToAnna();
    await pickWhy("Active deal");
    await screen.findByRole("combobox", { name: DEAL_PICK });

    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText("Choose the deal this follow-up is about."),
    ).toBeTruthy();
    expect(sent.some((s) => s.key === "POST /emails")).toBe(false);
  });

  it("asks nothing about a deal for another reason", async () => {
    const sent = openContactComposer();
    await writeToAnna();
    await pickWhy("Follow-up they requested");

    await waitFor(() =>
      expect(linksOf(sent, "POST /emails:preview")).toEqual([
        { entity_type: "contact", entity_id: "c-1" },
      ]),
    );
    expect(screen.queryByRole("combobox", { name: DEAL_PICK })).toBeNull();
  });

  it("sends a new email with a Re: subject as a new conversation", async () => {
    const sent = openContactComposer();
    await userEvent.type(screen.getByLabelText("To"), "anna@acme.test");
    await userEvent.tab();
    await userEvent.type(screen.getByLabelText("Subject"), "Re: Next steps");
    writeMessage("Body", "Picking this up again.");
    await pickWhy("Follow-up they requested");

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    await userEvent.click(await screen.findByRole("button", { name: "Send" }));

    await waitFor(() =>
      expect(sent.some((s) => s.key === "POST /emails")).toBe(true),
    );
    expect(
      sent.some(
        (s) =>
          s.key.includes(":preview") && s.key.startsWith("POST /activities/"),
      ),
    ).toBe(false);
    expect(sent.some((s) => s.key.endsWith("/send-email"))).toBe(false);
  });

  it.each([
    ["while the deals are loading", () => new Promise<Response>(() => {})],
    ["when the deals could not be loaded", () => jsonResponse({}, 500)],
    ["when not every deal is listed", () => jsonResponse(contactView(true))],
  ])("holds an Active deal send %s", async (_, view) => {
    const sent = openContactComposer(undefined, view);
    await writeToAnna();
    await pickWhy("Active deal");
    await screen.findByRole("combobox", { name: DEAL_PICK });

    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(
      await screen.findByText("Choose the deal this follow-up is about."),
    ).toBeTruthy();
    expect(sent.some((s) => s.key === "POST /emails")).toBe(false);
  });

  it("keeps the deal an edited scheduled message was filed under, and shows it", async () => {
    const sent = openContactComposer(scheduledEdit("deal-open"));

    expect(
      await screen.findByRole("combobox", { name: DEAL_PICK }),
    ).toBeTruthy();
    await waitFor(() =>
      expect(linksOf(sent, "POST /emails:preview")).toEqual([
        { entity_type: "contact", entity_id: "c-1" },
        { entity_type: "deal", entity_id: "deal-open" },
      ]),
    );
  });

  it("drops a restored deal the contact no longer sits on, and asks again", async () => {
    const sent = openContactComposer(scheduledEdit("deal-left"));
    await screen.findByRole("combobox", { name: DEAL_PICK });

    await waitFor(() =>
      expect(linksOf(sent, "POST /emails:preview")).toEqual([
        { entity_type: "contact", entity_id: "c-1" },
      ]),
    );
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(
      await screen.findByText("Choose the deal this follow-up is about."),
    ).toBeTruthy();
    expect(sent.some((s) => s.key === "POST /emails")).toBe(false);
  });
});

function scheduledEdit(dealId: string): EditSeed {
  return {
    anchor: { type: "contact", id: "c-1" },
    replyTo: "",
    to: ["anna@acme.test"],
    cc: [],
    bcc: [],
    subject: "Next steps",
    body: "As agreed, the next steps.",
    html: "",
    files: [],
    draftRef: "",
    sendAt: "",
    context: "active_deal_followup",
    dealId,
    projectId: "",
  };
}

it("saves the chosen deal and uses the same filing after reopening", async () => {
  const user = userEvent.setup();
  let saved: object | undefined;
  const sent: Sent = stubRoutes({
    "GET /contacts/c-1/360": () => jsonResponse(contactView()),
    "GET /mail-drafts": () =>
      saved ? jsonResponse(saved) : jsonResponse({}, 404),
    "PUT /mail-drafts": () => {
      const call = sent
        .filter((item) => item.key === "PUT /mail-drafts")
        .at(-1);
      const body = call?.body;
      if (!body || typeof body !== "object")
        throw new Error("No saved draft request");
      saved = { ...body, id: "draft-one", version: 1, agent_drafted: false };
      return jsonResponse(saved);
    },
    "POST /emails": () => jsonResponse({ id: "sent-1" }, 202),
  });
  const composer = () => (
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />
  );
  const first = render(composer());
  await writeToAnna();
  await pickWhy("Active deal");
  await pickOption(
    user,
    await screen.findByRole("combobox", { name: DEAL_PICK }),
    "Deal: Acme Renewal",
  );
  await user.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() =>
    expect(saved).toMatchObject({
      editor_state: {
        filing: [{ anchor_type: "deal", anchor_id: "deal-open" }],
      },
    }),
  );
  first.unmount();
  render(composer());
  await screen.findByText("Saved draft restored");
  expect(
    (await screen.findByRole("combobox", { name: DEAL_PICK })).textContent,
  ).toContain("Acme Renewal");
  const expected = [
    { entity_type: "contact", entity_id: "c-1" },
    { entity_type: "deal", entity_id: "deal-open" },
  ];
  await waitFor(() =>
    expect(linksOf(sent, "POST /emails:preview")).toEqual(expected),
  );
  expect(sent.filter((item) => item.key === "POST /emails")).toHaveLength(0);
  const sendButton = screen.getByRole("button", { name: "Send" });
  await waitFor(() => expect(sendButton.hasAttribute("disabled")).toBe(false));
  await user.click(sendButton);
  await waitFor(() => expect(linksOf(sent, "POST /emails")).toEqual(expected));
  expect(sent.filter((item) => item.key === "POST /emails")).toHaveLength(1);
});

it("keeps an explicit cleared filing instead of reviving an agent's old deal", async () => {
  const sent = stubRoutes({
    "GET /contacts/c-1/360": () => jsonResponse(contactView()),
    "GET /mail-drafts": () =>
      jsonResponse({
        id: "draft-one",
        anchor_type: "contact",
        anchor_id: "c-1",
        version: 2,
        to: ["anna@acme.test"],
        cc: [],
        bcc: [],
        subject: "Next steps",
        body: "The latest words.",
        communication_context: "requested_followup",
        agent_drafted: false,
        grounding: [{ anchor_type: "deal", anchor_id: "deal-open" }],
        editor_state: { filing: [] },
      }),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByText("Saved draft restored");
  await waitFor(() =>
    expect(linksOf(sent, "POST /emails:preview")).toEqual([
      { entity_type: "contact", entity_id: "c-1" },
    ]),
  );
  expect(screen.queryByRole("combobox", { name: DEAL_PICK })).toBeNull();
});

it("keeps the saved deal while its visible choices are still loading", async () => {
  const user = userEvent.setup();
  const sent: Sent = stubRoutes({
    "GET /contacts/c-1/360": () => new Promise<Response>(() => {}),
    "GET /mail-drafts": () =>
      jsonResponse({
        id: "draft-one",
        anchor_type: "contact",
        anchor_id: "c-1",
        version: 2,
        to: ["anna@acme.test"],
        cc: [],
        bcc: [],
        subject: "Next steps",
        body: "Saved words.",
        communication_context: "active_deal_followup",
        agent_drafted: false,
        editor_state: {
          filing: [{ anchor_type: "deal", anchor_id: "deal-open" }],
        },
      }),
    "PUT /mail-drafts": () =>
      jsonResponse({
        id: "draft-one",
        anchor_type: "contact",
        anchor_id: "c-1",
        version: 3,
        to: ["anna@acme.test"],
        cc: [],
        bcc: [],
        subject: "Next steps",
        body: "Edited while loading.",
      }),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
  );
  await screen.findByText("Saved draft restored");
  await waitFor(() =>
    expect(linksOf(sent, "POST /emails:preview")).toEqual([
      { entity_type: "contact", entity_id: "c-1" },
    ]),
  );
  await user.click(screen.getByRole("button", { name: "Send" }));
  expect(sent.some((item) => item.key === "POST /emails")).toBe(false);
  writeMessage("Body", "Edited while loading.");
  await user.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() =>
    expect(
      sent.find((item) => item.key === "PUT /mail-drafts")?.body,
    ).toMatchObject({
      editor_state: {
        filing: [{ anchor_type: "deal", anchor_id: "deal-open" }],
      },
    }),
  );
});

it("saves a cleared deal from an older draft even when its words stay the same", async () => {
  const user = userEvent.setup();
  const onClose = vi.fn();
  const saved = {
    id: "draft-old",
    version: 3,
    anchor_type: "contact",
    anchor_id: "c-1",
    to: ["anna@acme.test"],
    cc: [],
    bcc: [],
    subject: "Next steps",
    body: "The current draft.",
    communication_context: "active_deal_followup",
    grounding: [{ anchor_type: "deal", anchor_id: "deal-open" }],
    agent_drafted: false,
  };
  const sent = stubRoutes({
    "GET /contacts/c-1/360": () => jsonResponse(contactView()),
    "GET /mail-drafts": () => jsonResponse(saved),
    "PUT /mail-drafts": () => jsonResponse({ ...saved, version: 4 }),
  });
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={onClose}
    />,
  );
  await screen.findByText("Saved draft restored");
  const deal = await screen.findByRole("combobox", { name: DEAL_PICK });
  expect(deal.textContent).toContain("Acme Renewal");
  await pickOption(user, deal, "No deal");
  await user.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(onClose).toHaveBeenCalled());
  const writes = sent.filter((call) => call.key === "PUT /mail-drafts");
  expect(writes).toHaveLength(1);
  expect(writes[0]?.body).toMatchObject({
    body: saved.body,
    editor_state: { filing: [] },
  });
  expect(writes[0]?.headers.get("If-Match")).toBe("3");
  expect(sent.some((call) => call.key === "POST /emails")).toBe(false);
});
