// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactActions } from "./contactactions";

afterEach(cleanup);

// Reachable both ways, so a header that drew a Call verb would show it.
const reachable: components["schemas"]["Contact360"] = {
  ...view,
  contact: {
    ...view.contact,
    phones: [
      {
        id: "ph-0",
        phone: "+49 30 1111 2222",
        phone_type: "work",
        is_primary: true,
        position: 0,
        source: "manual",
        captured_by: "human:u-1",
      },
    ],
  },
};

function mountHeader(contact = reachable.contact) {
  installFetchStub({
    "GET /me": meRoute(
      { contact: ["read", "update"], activity: ["create"] },
      { seat: "full" },
    ),
    "GET /channel-providers": () => jsonResponse({ data: [] }),
  });
  render(
    <StoryProviders>
      <ContactActions
        view={{ ...reachable, contact }}
        contactId={reachable.contact.id}
        onWrite={() => undefined}
        onResearch={() => undefined}
        onLogActivity={() => undefined}
        onAddTask={() => undefined}
      />
    </StoryProviders>,
  );
}

describe("the contact header", () => {
  it("leads with Email and leaves dialling to the number", () => {
    mountHeader();
    expect(
      screen.getByRole("button", { name: en["contact.action.email"] }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("link", { name: en["contact.action.call"] }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: en["contact.action.call"] }),
    ).toBeNull();
  });

  it("keeps meetings, Log activity and the menu, with Add task inside it", () => {
    mountHeader();
    expect(
      screen.queryByRole("button", { name: en["contact.action.addTask"] }),
    ).toBeNull();
    for (const verb of [
      en["contact.action.meetings"],
      en["log.title"],
      en["record.moreActions"],
    ]) {
      expect(screen.getByRole("button", { name: verb })).toBeTruthy();
    }
  });
});

it.each([false, undefined])(
  "refuses booking and logging without an attach right: %s",
  async (attachable) => {
    const user = userEvent.setup();
    mountHeader({ ...reachable.contact, attachable });
    const reason = await screen.findByText(en["contact.attachRefused"]);
    const log = screen.getByRole("button", { name: en["log.title"] });
    expect(log).toHaveProperty("disabled", true);
    expect(log.getAttribute("aria-describedby")).toBe(reason.id);
    await user.click(
      screen.getByRole("button", { name: en["record.moreActions"] }),
    );
    const book = screen.getByRole("button", { name: en["scheduling.new"] });
    expect(book).toHaveProperty("disabled", true);
    expect(book.getAttribute("aria-describedby")).toBe(reason.id);
    expect(
      screen.getByRole("button", { name: en["contact.action.addTask"] }),
    ).toHaveProperty("disabled", true);
  },
);

it("allows attaching to a readable contact even when its row cannot be edited", async () => {
  const user = userEvent.setup();
  mountHeader({ ...reachable.contact, writable: false, attachable: true });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: en["log.title"] }),
    ).toHaveProperty("disabled", false),
  );
  await user.click(
    screen.getByRole("button", { name: en["record.moreActions"] }),
  );
  expect(
    screen.getByRole("button", { name: en["scheduling.new"] }),
  ).toHaveProperty("disabled", false);
});
