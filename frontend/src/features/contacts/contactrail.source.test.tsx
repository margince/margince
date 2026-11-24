// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { type Contact360, ContactRail } from "./contactrail";

afterEach(cleanup);

const CONFIRMED: NonNullable<Contact360["privacy_confirmed_source"]> = {
  case_id: "case-source",
  source_kind: "in_person_permission",
  source_obtained_at: "2026-11-15T12:00:00Z",
  confirmed_at: "2026-11-16T12:00:00Z",
  confirmed_by: {
    actor_type: "human",
    actor_id: "human:u-admin",
    actor_name: "Demo Admin",
  },
};

function serve(contact: Contact360) {
  installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
  render(
    <StoryProviders>
      <ContactRail
        view={contact}
        consent={{
          guard: undefined,
          loading: false,
          failed: false,
          onRetry: () => undefined,
        }}
        writeRefusalId="write-refused"
      />
    </StoryProviders>,
  );
}

const sourceSection = () =>
  screen.queryByRole("region", { name: en["history.field.source"] });

describe("the Source section", () => {
  it("stays after the review closes and states the confirmed source", async () => {
    serve({
      ...view,
      privacy_source_reviews: [],
      privacy_confirmed_source: CONFIRMED,
    });

    const section = within(
      await screen.findByRole("region", { name: en["history.field.source"] }),
    );
    expect(section.getByText("Met face to face on 15/11/2026")).toBeTruthy();
    expect(
      section.getByText("Confirmed by Demo Admin on 16/11/2026"),
    ).toBeTruthy();
  });

  it("is left out while no source was confirmed and none waits for review", async () => {
    serve({
      ...view,
      privacy_source_reviews: [],
      privacy_confirmed_source: undefined,
    });

    await screen.findByRole("region", { name: en["contact.aside.consent"] });
    expect(sourceSection()).toBeNull();
  });
});
