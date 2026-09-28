// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { WhoCanSeePanel } from "./recordaccesspanel";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

type RecordAccess = components["schemas"]["RecordAccess"];
type Member = components["schemas"]["RecordAccessMember"];

const meta: Meta = {
  title: "Records/Who can see this record",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const CONTACT = "01a05500-0000-7000-8000-0000000000d1";

function member(id: string, name: string, over: Partial<Member>): Member {
  return {
    user_id: `01a05500-0000-7000-8000-00000000${id}`,
    display_name: name,
    group: "everyone",
    can_change: false,
    read_reasons: [{ code: "workspace_visible" }],
    change_reasons: [],
    ...over,
  };
}

const owner = member("0a01", "Alex Owner", {
  group: "owner",
  can_change: true,
  read_reasons: [{ code: "workspace_visible" }, { code: "owner" }],
  change_reasons: [{ code: "owner" }],
});
const lead = member("0a02", "Sam Lead", {
  can_change: true,
  change_reasons: [{ code: "same_team_as_owner" }],
});
const colleagues = [
  lead,
  member("0a03", "Kim Rep", {}),
  member("0a04", "Noor Rep", {}),
];

function answer(over: Partial<RecordAccess>): RecordAccess {
  return {
    visibility: "workspace",
    owner_id: owner.user_id,
    archived: false,
    detail: false,
    you: {
      can_change: true,
      read_reasons: owner.read_reasons,
      change_reasons: owner.change_reasons,
    },
    data: [owner, ...colleagues],
    page: { has_more: false, total: 4 },
    group_counts: { owner: 1, shared: 0, team_shared: 0, everyone: 3 },
    can_change_count: 2,
    team_access_count: 0,
    refresh_at: null,
    ...over,
  };
}

function Panel({ body }: Readonly<{ body: RecordAccess }>) {
  stubWithSession(
    { [`GET /contacts/${CONTACT}/access`]: () => jsonResponse(body) },
    { contact: ["read", "update"] },
  );
  return (
    <StoryProviders>
      <WhoCanSeePanel kind="contact" recordId={CONTACT} />
    </StoryProviders>
  );
}

/** A contact the whole company can open: the owner stands alone, and
 *  everyone else folds into one row the reader opens when they want it. */
export const OpenToTheCompany: Story = {
  render: () => <Panel body={answer({})} />,
};

/** A private contact with shares: no "everyone" row, every colleague listed,
 *  and a share that lapses says when. */
export const PrivateWithShares: Story = {
  render: () => (
    <Panel
      body={answer({
        visibility: "owner",
        data: [
          { ...owner, read_reasons: [{ code: "owner" }] },
          member("0a05", "Priya Shah", {
            group: "shared",
            read_reasons: [
              {
                code: "user_share",
                access: "read",
                expires_at: "2026-10-30T12:00:00Z",
              },
            ],
          }),
          member("0a06", "Mor Adler", {
            group: "team_shared",
            can_change: true,
            read_reasons: [{ code: "team_share", access: "write" }],
            change_reasons: [{ code: "write_share", access: "write" }],
          }),
        ],
        page: { has_more: false, total: 3 },
        group_counts: { owner: 1, shared: 1, team_shared: 1, everyone: 0 },
        refresh_at: "2026-10-30T12:00:00Z",
      })}
    />
  ),
};

/** What an administrator reads: roles beside each name, and the team behind
 *  a team share named. */
export const AsAnAdministrator: Story = {
  render: () => (
    <Panel
      body={answer({
        detail: true,
        data: [
          { ...owner, roles: ["rep"] },
          member("0a06", "Mor Adler", {
            group: "team_shared",
            roles: ["manager"],
            read_reasons: [
              { code: "team_share", access: "read", team_name: "Deal Desk" },
            ],
          }),
          { ...colleagues[1], roles: ["read_only"] },
        ],
        group_counts: { owner: 1, shared: 0, team_shared: 1, everyone: 1 },
      })}
    />
  ),
};

/** A reader who may open the contact and not change it, on an archived
 *  record. */
export const ReadOnlyAndArchived: Story = {
  render: () => (
    <Panel
      body={answer({
        archived: true,
        data: [owner, ...colleagues].map((m) => ({
          ...m,
          can_change: false,
          change_reasons: [],
        })),
        can_change_count: 0,
        you: {
          can_change: false,
          read_reasons: [{ code: "workspace_visible" }],
          change_reasons: [],
        },
      })}
    />
  ),
};

/** What a rep reads when a team adds access: colleagues are shown by what
 *  holds without their teams, and the ones a team adds are counted. */
export const AsARepWithTeamAccess: Story = {
  render: () => (
    <Panel
      body={answer({
        data: [owner, ...colleagues.slice(1)],
        group_counts: { owner: 1, shared: 0, team_shared: 0, everyone: 2 },
        can_change_count: 1,
        team_access_count: 2,
      })}
    />
  ),
};
