// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import type { RecordAssignment, RecordRole } from "./recordassignments.queries";
import { RecordTeamAssign } from "./recordteamassign";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Naming who is responsible for a record, and moving that responsibility.
//
// One modal for both verbs, so the frames worth a picture are the two titles
// and what each seeds — a new assignment starts empty, a move starts from the
// colleague and role already on the row.
//
// The third is the rule the picker keeps rather than discovering through a
// refused save: the role list is what the server would accept for THIS record
// type and THIS kind of assignee, so switching from a colleague to a team can
// leave nothing to pick. That state has to read as "no role applies here" and
// not as a broken control, which is why it is a story.

const RECORD_ID = "11111111-1111-1111-1111-111111111111";

const role = (over: Partial<RecordRole>): RecordRole =>
  ({
    id: "r1",
    key: "account_manager",
    label: "Account manager",
    record_types: ["deal"],
    assignee_kinds: ["user"],
    sort_order: 1,
    active: true,
    system: true,
    version: 1,
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    ...over,
  }) as RecordRole;

const ROLES: RecordRole[] = [
  role({}),
  role({
    id: "r2",
    key: "delivery_lead",
    label: "Delivery lead",
    assignee_kinds: ["user", "team"],
    sort_order: 2,
  }),
];

const existing: RecordAssignment = {
  id: "a1",
  record_type: "deal",
  record_id: RECORD_ID,
  subject_kind: "user",
  subject_id: "u1",
  subject_name: "Mara Feld",
  role_id: "r1",
  role_key: "account_manager",
  role_label: "Account manager",
  role_active: true,
  version: 1,
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-01T10:00:00Z",
} as RecordAssignment;

function assign(roles: RecordRole[], moving?: RecordAssignment) {
  return () => {
    installFetchStub({
      "GET /record-roles": () => jsonResponse({ data: roles }),
      "GET /users": () =>
        jsonResponse({
          data: [
            {
              id: "u2",
              display_name: "Jonas Weiler",
              email: "jonas@example.test",
              is_agent: false,
              status: "active",
            },
          ],
          page: { has_more: false },
        }),
      "GET /teams": () =>
        jsonResponse({ data: [{ id: "t1", name: "Platform support" }] }),
    });
    return (
      <StoryProviders>
        <RecordTeamAssign
          open
          onClose={() => {}}
          recordType="deal"
          recordId={RECORD_ID}
          existing={moving}
        />
      </StoryProviders>
    );
  };
}

// Modal portals to document.body, so `#storybook-root` holds the preview
// decorator and nothing else however well the modal renders. Each frame drives
// a play that names what it expects, so a modal that never mounted fails the
// render gate instead of passing it.
const meta: Meta<typeof RecordTeamAssign> = {
  title: "Records/Shared/Name who is responsible",
  component: RecordTeamAssign,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Who");
  },
};
export default meta;

type Story = StoryObj<typeof RecordTeamAssign>;

/** Nobody named yet: a colleague to find, a role to pick, and no save until both. */
export const NamingSomebody: Story = { render: assign(ROLES) };

/**
 * Moving a responsibility. The colleague and the role already on the row seed
 * the form, so a reader changing one does not restate the other.
 */
export const ChangingWhoHoldsIt: Story = { render: assign(ROLES, existing) };

/**
 * A team, where only one of the two roles applies. The role clears with the
 * switch rather than being carried into a list that no longer offers it — left
 * standing it would submit a combination the server refuses.
 */
export const SwitchedToATeam: Story = {
  render: assign(ROLES, existing),
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(await dialog.findByRole("button", { name: "Team" }));
    await dialog.findByRole("searchbox", { name: "Find a team" });
  },
};
