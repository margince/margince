/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactTagsSection } from "./contactdetails";

afterEach(cleanup);

// The verb belongs to the panel, not to each host that mounts it. A contact
// page once shipped with tags a reader could see and no way to add one. The add
// button lived in the company wrapper and the contact mount never placed it.
// These drive the real mount, so they fail if it stops computing `canEdit`
// correctly. A literal prop would only prove the panel obeys whatever it gets.
describe("the contact mount offers the verb the panel draws", () => {
  const CONTACT = "01a06151-0000-7000-8000-000000000002";

  // The tags read the panel waits on. A bare panel with nothing to show and
  // nothing to offer draws no text, so the read is the signal it has settled.
  let tagsRead = false;
  async function settled() {
    await waitFor(() => expect(tagsRead).toBe(true));
    await act(async () => {});
  }

  function mountContact(
    contact: Record<string, unknown>,
    grants: Record<string, string[]> = { contact: ["update"] },
    seat: "full" | "read" = "full",
  ) {
    tagsRead = false;
    installFetchStub({
      "GET /me": meRoute(grants as never, { seat }),
      [`GET /records/contact/${CONTACT}/tags`]: () => {
        tagsRead = true;
        return jsonResponse({ data: [], withheld: false });
      },
    });
    render(
      <StoryProviders>
        <ContactTagsSection contact={{ id: CONTACT, ...contact } as never} />
      </StoryProviders>,
    );
  }

  // The control: without it, a verb that never renders would pass every test
  // below for the wrong reason.
  it("offers the verb on a contact the seat may write", async () => {
    mountContact({ writable: true });
    expect(
      await screen.findByRole("button", { name: en["tags.add"] }),
    ).toBeInTheDocument();
  });

  // The row axis. A rep holding `contact.update` on the OBJECT still may not
  // write a colleague's contact, and the server stamps that as `writable`.
  it("offers no verb on a contact this reader may not write", async () => {
    mountContact({ writable: false });
    await settled();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // The seat axis. A read seat is refused by the licensing middleware before
  // RBAC is consulted. A verb offered to one is a control whose save cannot
  // succeed.
  it("offers no verb to a read seat", async () => {
    mountContact({ writable: true }, { contact: ["update"] }, "read");
    await settled();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // The object axis, for completeness: all three are necessary and none of the
  // others would catch a mount that dropped this one.
  it("offers no verb to a seat holding no update grant", async () => {
    mountContact({ writable: true }, {});
    await settled();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });
});
