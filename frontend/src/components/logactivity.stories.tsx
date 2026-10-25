// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckSquare, FileText } from "lucide-react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { LogActivity } from "./logactivity";
import { LogActivityAction } from "./logactivity.action";

// The log-an-activity form embedded in every 360 (contact/company/deal/lead).
// It reads GET /me only for the caller's own grant; the form itself never
// fetches.
function admin() {
  return () =>
    jsonResponse({
      user: { id: "u1", email: "ada@acme.test", display_name: "Ada" },
      roles: ["admin"],
      teams: [],
    });
}

const meta: Meta<typeof LogActivity> = {
  title: "Patterns/Log activity",
  component: LogActivity,
};
export default meta;
type Story = StoryObj<typeof LogActivity>;

export const Default: Story = {
  render: () => {
    installFetchStub({ "GET /me": admin() });
    return (
      <StoryProviders>
        <LogActivity entityType="contact" entityId="p1" />
      </StoryProviders>
    );
  },
};

// Opened on the kind the caller named. A rep sent here to log a call attempt
// arrives on the call form rather than on a note they have to change — the
// date field is the day it happened, the same field a note gets, because a
// call is not owed.
export const OpenedOnACall: Story = {
  render: () => {
    installFetchStub({ "GET /me": admin() });
    return (
      <StoryProviders>
        <LogActivity entityType="lead" entityId="l1" askedKind="call" />
      </StoryProviders>
    );
  },
};

// A task is the one kind held by a colleague, so it is the only one that draws
// the assignee picker — beside the due date, defaulting to Unassigned. The
// roster it offers is the workspace's contacts less agent seats: the walk stubbed
// here carries a Runner Bot the picker must not list, because the server
// refuses one as an assignee.
export const OpenedOnATask: Story = {
  render: () => {
    installFetchStub({
      "GET /me": admin(),
      "GET /users": () =>
        jsonResponse({
          data: [
            { id: "u1", display_name: "Ada Ops", is_agent: false },
            { id: "u2", display_name: "Priya Lead", is_agent: false },
            { id: "agent-1", display_name: "Runner Bot", is_agent: true },
          ],
          page: { next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <LogActivity entityType="contact" entityId="p1" askedKind="task" />
      </StoryProviders>
    );
  },
};

// The same form reached from a record header, where it is a TRIGGER rather
// than a panel: two of them side by side, each naming its own verb and each
// leading with the glyph the contact header already gives it. The words stay —
// a page and a tick box do not say "write down what happened" and "file a task"
// on their own, and a reader would have to hover to tell the pair apart. The
// button sizes the glyph, so neither call site names a size.
export const HeaderTriggers: Story = {
  render: () => {
    installFetchStub({ "GET /me": admin() });
    return (
      <StoryProviders>
        <div style={{ display: "flex", gap: "var(--gapActions)" }}>
          <LogActivityAction
            entityType="company"
            entityId="o1"
            triggerIcon={<FileText aria-hidden="true" />}
          />
          <LogActivityAction
            entityType="company"
            entityId="o1"
            askedKind="task"
            triggerIcon={<CheckSquare aria-hidden="true" />}
          />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["log.title"],
      }),
    );
    const drawer = await within(document.body).findByRole("dialog", {
      name: en["log.title"],
    });
    const heading = await within(drawer).findByRole("heading", {
      name: en["log.title"],
    });
    // The overlay arrives from opacity 0, so visibility holds once it settles.
    await waitFor(() => expect(heading).toBeVisible());
  },
};

export const WithFollowUp: Story = {
  ...Default,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("checkbox", { name: en["log.addFollowUp"] }),
    );
    await userEvent.type(
      canvas.getByRole("textbox", { name: en["log.followUpSubject"] }),
      "Send revised quote",
    );
    await expect(
      canvas.getByRole("textbox", { name: en["log.followUpSubject"] }),
    ).toBeVisible();
  },
};
export const WithFollowUpDark: Story = {
  ...WithFollowUp,
  globals: { theme: "dark" },
};

// Files picked while logging wait in the form, each removable, and are uploaded
// against the activity once it is created. The hint states the installation's
// limit.
export const WithFiles: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["create", "update"] }),
      "GET /installation/settings": () =>
        jsonResponse({ max_upload_bytes: 25_000_000 }),
    });
    return (
      <StoryProviders>
        <LogActivity entityType="contact" entityId="p1" askedKind="call" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.upload(await canvas.findByLabelText(en["log.files"]), [
      new File(["prices"], "price-list-2026.pdf", { type: "application/pdf" }),
      new File(["agenda"], "agenda.docx"),
    ]);
    await expect(
      await canvas.findByRole("button", { name: /price-list-2026\.pdf/ }),
    ).toBeVisible();
  },
};
export const WithFilesDark: Story = {
  ...WithFiles,
  globals: { theme: "dark" },
};

// A workspace that added a field to activities: the form offers it under the
// body, for whichever kind is being logged.
const visitType = {
  id: "cf-visit-type",
  object: "activity",
  label: "Visit type",
  slug: "visit_type",
  type: "picklist",
  options: ["On site", "Remote"],
  status: "active",
  column_name: "cf_visit_type",
  show_in_create: true,
  created_by: "human:u-1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

export const WithCustomFields: Story = {
  render: () => {
    installFetchStub({
      "GET /me": admin(),
      "GET /custom-fields": () =>
        jsonResponse({ data: [visitType], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <LogActivity entityType="contact" entityId="p1" askedKind="call" />
      </StoryProviders>
    );
  },
};
export const WithCustomFieldsDark: Story = {
  ...WithCustomFields,
  globals: { theme: "dark" },
};
