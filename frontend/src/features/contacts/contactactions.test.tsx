// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
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

// Reachable both ways, so a header that still drew either verb would show it.
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

function mountHeader() {
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
        view={reachable}
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
  it("leaves writing and calling to the address and the number", () => {
    mountHeader();
    for (const verb of [
      en["contact.action.call"],
      en["compose.writeEmail"],
      en["record.verb.email"],
    ]) {
      expect(screen.queryByRole("link", { name: verb })).toBeNull();
      expect(screen.queryByRole("button", { name: verb })).toBeNull();
    }
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
