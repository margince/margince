// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { captureReviewTarget } from "../../lib/capturereview";
import { routeHash } from "../../lib/router";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { TimelineActions } from "./timelineactions";

type Activity = components["schemas"]["Activity"];

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

const LINK = "Show held mail with this contact";

const held: Activity = {
  id: "mail-one",
  kind: "email",
  occurred_at: "2026-09-01T09:00:00Z",
  occurred_precision: "instant",
  created_at: "2026-09-01T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
  captured_by: "connector:gmail:owner",
  source: "gmail",
  is_done: false,
  offers_email: false,
  audience: "participants",
  audience_reason: "pending_verdict",
  content_state: "available",
};

function renderRow(
  activity: Activity,
  entityType: "contact" | "deal" = "contact",
) {
  stubWithSession({}, { activity: ["read"] });
  render(
    <StoryProviders>
      <TimelineActions
        activity={activity}
        entityType={entityType}
        entityId="contact-one"
      />
    </StoryProviders>,
  );
}

it("links a held mail to the held list narrowed to the contact", async () => {
  renderRow(held);
  const user = userEvent.setup();
  await user.click(await screen.findByRole("link", { name: LINK }));

  const [path, query] = window.location.hash.split("?");
  const params = new URLSearchParams(query);
  expect(path).toBe(routeHash(captureReviewTarget("held").route));
  expect(params.get("review")).toBe("held");
  expect(params.get("contact_id")).toBe("contact-one");
});

it.each<[string, Activity]>([
  ["withheld from the reader", { ...held, content_state: "withheld" }],
  ["shared with the workspace", { ...held, audience: "workspace" }],
  ["limited by a person", { ...held, audience_reason: "manual" }],
  ["limited by nothing derived", { ...held, audience_reason: null }],
  ["not a mail", { ...held, kind: "message" }],
])("offers no held-list link on a row %s", async (_, activity) => {
  renderRow(activity);
  await screen.findByRole("button", { name: /more actions/i });
  expect(screen.queryByRole("link", { name: LINK })).toBeNull();
});

it("offers no held-list link outside a contact's timeline", async () => {
  renderRow(held, "deal");
  await screen.findByRole("button", { name: /more actions/i });
  expect(screen.queryByRole("link", { name: LINK })).toBeNull();
});
