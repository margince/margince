// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, waitFor, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ContactSourceReview } from "./contactsourcereview";

const review = {
  id: "case-source",
  contact_id: "contact-source",
  contact_name: "Dana Example",
  rule: "unassessed",
  state: "source_review",
  due_at: null,
  attempts: 0,
  created_at: "2026-01-01T10:00:00Z",
  allowed_routes: [],
  acquisition: {
    kind: "unknown_legacy",
    captured_at: "2025-11-03T09:00:00Z",
    captured_by: "system",
  },
};

function officer(grants: ("read" | "update")[]) {
  stubWithSession(
    {
      "GET /privacy/notice-case-reviews/case-source/source": () =>
        jsonResponse({ items: [] }),
      "GET /privacy/notice-case-reviews/case-source": () =>
        jsonResponse(review),
    },
    { privacy_request: grants },
  );
}

const meta: Meta<typeof ContactSourceReview> = {
  title: "Records/Contact 360/Source review",
  component: ContactSourceReview,
  decorators: [
    (Story) => {
      officer(["read", "update"]);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { reviews: ["case-source"] },
};
export default meta;
type Story = StoryObj<typeof ContactSourceReview>;
export const SourceReview: Story = {};

export const MultipleSourceReviews: Story = {
  args: { reviews: ["case-source", "case-source-2"] },
};

/** A reader who may not assess the source is told who does. */
export const ReadOnly: Story = {
  decorators: [
    (Story) => {
      officer(["read"]);
      return <Story />;
    },
  ],
};

/** The drawer over the record: the reason, what is on record, the question. */
export const Drawer: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Review source",
      }),
    );
  },
};

const confirmed = {
  case_id: "case-source",
  source_kind: "in_person_permission",
  source_obtained_at: "2026-11-15T12:00:00Z",
  confirmed_at: "2026-11-16T12:00:00Z",
  confirmed_by: {
    actor_type: "human" as const,
    actor_id: "human:u-admin",
    actor_name: "Demo Admin",
  },
};

/** The confirmed source: what, when it was obtained, and who confirmed it. */
export const ConfirmedSource: Story = {
  args: { reviews: [], confirmed },
};

/** Confirmed with no acquisition date, and no confirmer the server could name. */
export const ConfirmedWithoutDate: Story = {
  args: {
    reviews: [],
    confirmed: {
      ...confirmed,
      source_kind: "event_or_form",
      source_obtained_at: null,
      confirmed_by: undefined,
    },
  },
};

/** One source confirmed and another still waiting for review. */
export const ConfirmedAndOpen: Story = {
  args: { reviews: ["case-source"], confirmed },
};

/** Confirm source saved: the drawer closes and a short notice says so. */
export const ConfirmedNotice: Story = {
  decorators: [
    (Story) => {
      stubWithSession(
        {
          "GET /privacy/notice-case-reviews/case-source/source": () =>
            jsonResponse({ items: [] }),
          "GET /privacy/notice-case-reviews/case-source": () =>
            jsonResponse(review),
          "POST /privacy/notice-case-reviews/case-source/source/preview": () =>
            jsonResponse({
              preview_token: "a".repeat(64),
              rule: "none",
              notice_required: false,
              due_at: null,
              timezone: "UTC",
              deadline_is_upper_bound: false,
            }),
          "POST /privacy/notice-case-reviews/case-source/source": () =>
            jsonResponse({ ...review, state: "not_required", rule: "none" }),
        },
        { privacy_request: ["read", "update"] },
      );
      return (
        <ToastProvider>
          <Story />
          <ToastRegion />
        </ToastProvider>
      );
    },
  ],
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["notice.source.review"],
      }),
    );
    const drawer = within(await screen.findByRole("dialog"));
    await userEvent.click(
      drawer.getByRole("radio", { name: en["notice.source.answer.met"] }),
    );
    await userEvent.type(
      drawer.getByRole("textbox", { name: en["noticeDuty.obtainedOn"] }),
      "15/11/2026",
    );
    await userEvent.tab();
    await userEvent.type(
      drawer.getByRole("textbox", { name: en["notice.source.evidence"] }),
      "Met at the trade fair stand.",
    );
    const confirm = drawer.getByRole("button", {
      name: en["notice.source.confirm"],
    });
    await waitFor(() => {
      if (confirm.hasAttribute("disabled")) throw new Error("preview pending");
    });
    await userEvent.click(confirm);
    await screen.findByText(en["notice.source.saved"]);
  },
};
