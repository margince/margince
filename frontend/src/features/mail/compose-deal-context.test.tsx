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
// An Active deal reason is proved by a deal, so a contact's composer offers
// the contact's open deals beside it and files the message under the one the
// rep chose. The preview and the send carry the same links, because the
// engine reads the deal from them. These tests assert the request bodies.

const DEAL_PICK = "Deal this follow-up is about";

// One open deal, one lost one, and the open one seated twice.
const CONTACT_VIEW = {
  as_of: "2026-09-01T09:00:00Z",
  sections_omitted: [],
  contact: { id: "c-1", full_name: "Anna Weber" },
  deal_roles: {
    data: [
      {
        relationship_id: "r-1",
        deal_id: "deal-open",
        deal_title: "Acme Renewal",
        deal_status: "open",
        role: "champion",
      },
      {
        relationship_id: "r-2",
        deal_id: "deal-open",
        deal_title: "Acme Renewal",
        deal_status: "open",
        role: "economic_buyer",
      },
      {
        relationship_id: "r-3",
        deal_id: "deal-lost",
        deal_title: "Acme Pilot",
        deal_status: "lost",
        role: "champion",
      },
    ],
    page: { has_more: false },
  },
};

type Sent = ReturnType<typeof stubRoutes>;

function linksOf(sent: Sent, key: string) {
  const bodies = sent.filter((s) => s.key === key).map((s) => s.body);
  return (bodies.at(-1) as { links?: unknown } | undefined)?.links;
}

function openContactComposer(edit?: EditSeed) {
  const sent = stubRoutes({
    "GET /contacts/c-1/360": () => jsonResponse(CONTACT_VIEW),
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

  it("keeps the deal an edited scheduled message was filed under, and shows it", async () => {
    const sent = openContactComposer({
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
      dealId: "deal-open",
      projectId: "",
    });

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
});
