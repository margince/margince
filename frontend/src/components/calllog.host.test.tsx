/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { CallAction } from "../design-system/callaction";
import { telUri } from "../format/contacturi";
import { useMe } from "../lib/session";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { CallLogHost } from "./calllog.host";

afterEach(cleanup);

const DIALS = "+49 30 1234 5678";
const DANA = { entityType: "contact", entityId: "p-1" } as const;

// No press before the seat answers: a refusal must not pass on a /me that
// never arrived.
function SessionAnswered() {
  return useMe().data ? <p>session answered</p> : null;
}

async function mountHost(seat: "full" | "read" = "full") {
  const posted: unknown[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"], contact: ["read"] }, { seat }),
    "GET /contacts/p-1": () =>
      jsonResponse({ id: "p-1", full_name: "Dana Buyer" }),
    "POST /activities": (body) => {
      posted.push(body);
      return jsonResponse(
        { id: "a-9", kind: "call", occurred_at: "2026-09-08T09:00:00Z" },
        201,
      );
    },
  });
  render(
    <StoryProviders>
      <CallLogHost>
        <SessionAnswered />
        <CallAction appearance="value" number={DIALS} record={DANA} />
      </CallLogHost>
    </StoryProviders>,
  );
  await screen.findByText("session answered");
  return { posted, link: screen.getByRole("link", { name: DIALS }) };
}

describe("a call started under the shell's host", () => {
  it("still dials, and opens Log activity on Call", async () => {
    const { link } = await mountHost();

    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    expect(fireEvent.click(link)).toBe(true);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    expect(
      within(dialog).getByRole("combobox", { name: "Type" }).textContent,
    ).toContain("Call");
  });

  it("files the saved call on the number's contact", async () => {
    const user = userEvent.setup();
    const { posted, link } = await mountHost();

    await user.click(link);
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Subject" }),
      "No answer",
    );
    await user.click(within(dialog).getByRole("button", { name: "Log" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      kind: "call",
      subject: "No answer",
      links: [{ entity_type: "contact", entity_id: "p-1" }],
    });
  });

  it("opens from the keyboard, takes focus, and records nothing when closed", async () => {
    const user = userEvent.setup();
    const { posted, link } = await mountHost();

    link.focus();
    await user.keyboard("{Enter}");
    const dialog = await screen.findByRole("dialog", { name: "Log activity" });
    await waitFor(() =>
      expect(dialog.contains(document.activeElement)).toBe(true),
    );
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(link);
    expect(posted).toHaveLength(0);
  });

  it("only dials for a read seat, which may not log activity", async () => {
    const { link } = await mountHost("read");

    expect(fireEvent.click(link)).toBe(true);
    expect(link.getAttribute("href")).toBe(telUri(DIALS));
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
