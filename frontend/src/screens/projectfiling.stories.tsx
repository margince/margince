// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ProjectFilingModal } from "./projectfiling";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type ProjectFiling = components["schemas"]["ProjectFiling"];

// Taking an activity back out of a project. The server decides whether the undo
// is allowed, and the dialog draws its verdict: the form when the filing is the
// only thing keeping the activity, the rule in the way when it is not.

const FILED: ProjectFiling = {
  filed: true,
  projects: [{ name: "ERP rollout", qualified_at: "2026-09-01T09:00:00Z" }],
  undoable: true,
  undone: [],
};

function dialog(filing: ProjectFiling) {
  return () => {
    installFetchStub({
      "GET /activities/a-1/project-filing": () => jsonResponse(filing),
    });
    return (
      <StoryProviders>
        <ProjectFilingModal
          activityId="a-1"
          projectId="p-1"
          open
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ProjectFilingModal> = {
  title: "Records/Record 360/Undo project filing",
  component: ProjectFilingModal,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectFilingModal>;

/** The only thing keeping the activity is the filing: a reason is required. */
export const Undoable: Story = { render: dialog(FILED) };

/** A won deal still qualifies the activity, so the dialog names the rule and
 *  offers no form. */
export const KeptByAnotherBasis: Story = {
  render: dialog({
    ...FILED,
    undoable: false,
    refusal: { code: "other_basis_remains", message: "" },
  }),
};

/** A statutory hold has started; it never shortens. */
export const Restricted: Story = {
  render: dialog({
    ...FILED,
    undoable: false,
    refusal: { code: "restricted", message: "" },
  }),
};

/** Already undone: the audit entry reads where the activity is. */
export const AlreadyUndone: Story = {
  render: dialog({
    filed: false,
    projects: [],
    undoable: false,
    undone: [
      {
        id: "d1d1d1d1-0000-4000-8000-000000000001",
        at: "2026-09-02T10:30:00Z",
        by_name: "Ada Admin",
        reason: "The assistant filed the wrong thread.",
        projects: ["ERP rollout"],
      },
    ],
  }),
};

/** A legal hold sits on a linked record; the retention mark stays until it lifts. */
export const LegalHold: Story = {
  render: dialog({
    ...FILED,
    undoable: false,
    refusal: { code: "legal_hold", message: "" },
  }),
};

/** The project is out of this member's sight: it is unnamed, and the undo is
 *  left to somebody who can see it. */
export const HiddenProject: Story = {
  render: dialog({
    ...FILED,
    projects: [
      { name: "", hidden: true, qualified_at: "2026-09-01T09:00:00Z" },
    ],
    undoable: false,
    refusal: { code: "hidden_project", message: "" },
  }),
};
