// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactSourceReview } from "./contactsourcereview";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function reader(grants: ("read" | "update")[]) {
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ items: [] }),
      "GET /privacy/notice-case-reviews/case-source": () =>
        jsonResponse({
          id: "case-source",
          contact_id: "contact-source",
          rule: "unassessed",
          state: "source_review",
          due_at: null,
          attempts: 0,
          created_at: "2026-01-01T10:00:00Z",
          allowed_routes: [],
        }),
    },
    { privacy_request: grants, contact: ["read"] },
  );
  render(
    <StoryProviders>
      <SessionLoaded />
      <ContactSourceReview reviews={["case-source"]} />
    </StoryProviders>,
  );
}

// Every grant reads false until /me answers, so a missing button proves
// nothing before then. This shows once the session has loaded.
function SessionLoaded() {
  const read = useCan("privacy_request", "read");
  const update = useCan("privacy_request", "update");
  const loaded = read || update;
  return loaded ? <span>session loaded</span> : null;
}

it("opens the source question over the record for a reader who may assess it", async () => {
  reader(["read", "update"]);
  const user = userEvent.setup();

  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );

  const drawer = within(screen.getByRole("dialog"));
  expect(
    drawer.getByRole("group", { name: en["notice.source.question"] }),
  ).toBeInTheDocument();
});

it("tells a reader who may not assess the source who does, with no button", async () => {
  reader(["read"]);

  await screen.findByText("session loaded");
  expect(
    await screen.findByText(en["notice.source.contactExplanation"]),
  ).toBeInTheDocument();
  expect(screen.getByText(en["notice.source.needsConfirmation"])).toBeVisible();
  expect(
    screen.queryByRole("button", { name: en["notice.source.review"] }),
  ).not.toBeInTheDocument();
});

// The server refuses an assessment without read as well as update, so an
// update-only grant must not get a button that ends in a refusal.
it("gives no button to a reader who holds update but not read", async () => {
  reader(["update"]);

  await screen.findByText("session loaded");
  expect(
    await screen.findByText(en["notice.source.contactExplanation"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: en["notice.source.review"] }),
  ).not.toBeInTheDocument();
});
