// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { useCan } from "../../lib/capability";
import { FIXTURE_TIMEZONE } from "../../testing/mefixture";
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

const CONFIRMED: components["schemas"]["ContactConfirmedSource"] = {
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

function confirmedOnly(confirmed: Partial<typeof CONFIRMED> = {}) {
  stubWithSession({}, { privacy_request: ["read"], contact: ["read"] });
  render(
    <StoryProviders>
      <ContactSourceReview
        reviews={[]}
        confirmed={{ ...CONFIRMED, ...confirmed }}
      />
    </StoryProviders>,
  );
}

it("states the confirmed source in the words of the question, when it was obtained, and who confirmed it", async () => {
  confirmedOnly();

  expect(
    await screen.findByText("Met face to face on 15/11/2026"),
  ).toBeVisible();
  expect(
    screen.getByText("Confirmed by Demo Admin on 16/11/2026"),
  ).toBeVisible();
  expect(
    screen.queryByText(en["notice.source.needsConfirmation"]),
  ).not.toBeInTheDocument();
});

it("leaves out a date nobody confirmed and a confirmer the server did not name", async () => {
  confirmedOnly({
    source_kind: "event_or_form",
    source_obtained_at: null,
    confirmed_by: undefined,
  });

  expect(await screen.findByText(en["notice.acq.eventOrForm"])).toBeVisible();
  expect(screen.getByText("Confirmed on 16/11/2026")).toBeVisible();
});

it("says the source was confirmed once the drawer saves it", async () => {
  const user = userEvent.setup();
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
      "POST /privacy/notice-case-reviews/case-source/source/preview": () =>
        jsonResponse({
          preview_token: "a".repeat(64),
          rule: "none",
          notice_required: false,
          due_at: null,
          timezone: FIXTURE_TIMEZONE,
          deadline_is_upper_bound: false,
        }),
      "POST /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ id: "case-source", state: "not_required" }),
    },
    { privacy_request: ["read", "update"], contact: ["read"] },
  );
  render(
    <StoryProviders>
      <ToastProvider>
        <ContactSourceReview reviews={["case-source"]} />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );

  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  const dialog = within(screen.getByRole("dialog"));
  await user.click(
    dialog.getByRole("radio", { name: en["notice.source.answer.met"] }),
  );
  await user.type(
    dialog.getByRole("textbox", { name: en["noticeDuty.obtainedOn"] }),
    "15/11/2026",
  );
  await user.tab();
  await user.type(
    dialog.getByRole("textbox", { name: en["notice.source.evidence"] }),
    "Met at the trade fair stand.",
  );
  const confirm = dialog.getByRole("button", {
    name: en["notice.source.confirm"],
  });
  await waitFor(() => expect(confirm).toBeEnabled());
  await user.click(confirm);

  expect(await screen.findByText(en["notice.source.saved"])).toBeVisible();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

it("says nothing when the drawer is left without a save", async () => {
  reader(["read", "update"]);
  const user = userEvent.setup();

  await user.click(
    await screen.findByRole("button", { name: en["notice.source.review"] }),
  );
  await user.click(
    within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }),
  );

  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(screen.queryByText(en["notice.source.saved"])).not.toBeInTheDocument();
});
