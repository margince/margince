/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { LogActivity } from "./logactivity";
import {
  type Captured,
  contact,
  createdActivity,
  jsonResponse,
  render,
  stubApi,
} from "./logactivity.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function provider(id: string, label: string) {
  return {
    provider: id,
    label,
    credential_model: "workspace_bot",
    supplies_transport: false,
    attachments: { max_bytes: 0, max_count: 0 },
  };
}

function mount(providers: ReturnType<typeof provider>[]) {
  const captured: Captured[] = [];
  stubApi(
    {
      "GET /contacts/p1": () => jsonResponse(contact),
      "GET /channel-providers": () => jsonResponse({ data: providers }),
      "POST /activities": createdActivity,
    },
    captured,
  );
  render(<LogActivity entityType="contact" entityId="p1" />);
  return captured;
}

// The option shows once the transport directory has answered, so the popup is
// opened once and the option awaited rather than picked at first paint.
async function pickMessage(user: UserEvent) {
  await user.click(screen.getByLabelText("Type"));
  await user.click(await screen.findByRole("option", { name: "Message" }));
}

const posted = (captured: Captured[]) =>
  captured.find((entry) => entry.key === "POST /activities")?.body;

describe("logging a chat conversation", () => {
  it("posts a message carried by the chat app the member picked", async () => {
    const captured = mount([
      provider("linkedin", "LinkedIn"),
      provider("whatsapp", "WhatsApp"),
    ]);
    const user = userEvent.setup();
    await pickMessage(user);
    writeMessage("Details", "Asked for the price list");
    const save = screen.getByRole("button", { name: "Log" });
    // Two chat apps and none picked: nothing says which one carried it yet.
    expect(save.hasAttribute("disabled")).toBe(true);

    await pickOption(user, screen.getByLabelText("Chat app *"), "WhatsApp");
    await user.click(save);

    await waitFor(() => expect(posted(captured)).toBeDefined());
    expect(posted(captured)).toMatchObject({
      kind: "message",
      channel_provider: "whatsapp",
      body: "Asked for the price list",
      links: [{ entity_type: "contact", entity_id: "p1" }],
      source: "manual",
    });
  });

  it("takes the one registered chat app without asking", async () => {
    const captured = mount([provider("linkedin", "LinkedIn")]);
    const user = userEvent.setup();
    await pickMessage(user);
    // A message says what happened through its words; no subject is needed.
    writeMessage("Details", "Accepted the connection");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted(captured)).toBeDefined());
    expect(posted(captured)).toMatchObject({
      kind: "message",
      channel_provider: "linkedin",
    });
  });

  it("offers no message where no chat app is registered to name", async () => {
    const captured = mount([]);
    const user = userEvent.setup();
    // The empty directory has answered, so the list below is its final one.
    await waitFor(() =>
      expect(
        captured.some((entry) => entry.key === "GET /channel-providers"),
      ).toBe(true),
    );
    await user.click(screen.getByLabelText("Type"));
    await screen.findByRole("option", { name: "Meeting" });
    expect(screen.queryByRole("option", { name: "Message" })).toBeNull();
  });

  it("sends no channel_provider for a note after a switch from Message", async () => {
    const captured = mount([provider("whatsapp", "WhatsApp")]);
    const user = userEvent.setup();
    await pickMessage(user);
    await pickOption(user, screen.getByLabelText("Type"), "Note");
    await user.type(screen.getByLabelText("Subject *"), "Recap");
    await user.click(screen.getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted(captured)).toBeDefined());
    expect(posted(captured)).toMatchObject({ kind: "note" });
    expect(posted(captured)).not.toHaveProperty("channel_provider");
  });
});
