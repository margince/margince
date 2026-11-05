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
