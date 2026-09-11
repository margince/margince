// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CheckSquare, FileText } from "lucide-react";
import { LogActivity, LogActivityAction } from "./logactivity";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The log-an-activity form embedded in every 360 (person/company/deal/lead).
// It reads GET /me only to gate itself on overlay mode (hidden there — its
// POST /activities writes a mirrored record, unsupported_by_sor); the form
// itself never fetches.
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

export const Native: Story = {
  render: () => {
    installFetchStub({ "GET /me": admin() });
    return (
      <StoryProviders>
        <LogActivity entityType="person" entityId="p1" />
      </StoryProviders>
    );
  },
};

// Overlay mode renders nothing — logging writes a mirrored record's native
// table directly, which the incumbent write-back seam does not shadow.
export const HiddenInOverlay: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          user: { id: "u1", email: "ada@acme.test", display_name: "Ada" },
          roles: ["admin"],
          teams: [],
          system_of_record: { mode: "overlay" },
        }),
    });
    return (
      <StoryProviders>
        <LogActivity entityType="person" entityId="p1" />
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
// roster it offers is the workspace's people less agent seats: the walk stubbed
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
        <LogActivity entityType="person" entityId="p1" askedKind="task" />
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
            triggerLabel="log.addTask"
            triggerIcon={<CheckSquare aria-hidden="true" />}
          />
        </div>
      </StoryProviders>
    );
  },
};
