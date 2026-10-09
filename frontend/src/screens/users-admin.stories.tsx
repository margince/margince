// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { UsersAdminCard } from "./users-admin";
import type { User } from "./users-members";

const ago = (hours: number) =>
  new Date(Date.now() - hours * 3_600_000).toISOString();
const EVERY_VERB: User["allowed_actions"] = [
  "change_role",
  "issue_password_link",
  "deactivate",
];

const TEAMS = [
  { id: "t-dach", name: "DACH Sales" },
  { id: "t-nordics", name: "Nordics" },
  { id: "t-key", name: "Key Accounts" },
  { id: "t-partners", name: "Partner Channel" },
  { id: "t-renewals", name: "Renewals and Expansion" },
];

const LARS: User = {
  id: "u-1",
  email: "lars@brandt.example",
  display_name: "Lars Brandt",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: false,
  roles: ["admin"],
  team_ids: ["t-dach"],
  last_active_at: ago(2),
  created_at: "2026-01-12T09:00:00Z",
  allowed_actions: ["issue_password_link"],
};

const DANA: User = {
  id: "u-2",
  email: "dana@brandt.example",
  display_name: "Dana Kessler",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: false,
  roles: ["rep"],
  team_ids: ["t-dach", "t-nordics"],
  last_active_at: ago(30),
  created_at: "2026-02-03T09:00:00Z",
  allowed_actions: EVERY_VERB,
};

const IVY: User = {
  id: "u-4",
  email: "ivy@brandt.example",
  display_name: "Ivy Lindqvist",
  timezone: "Europe/Berlin",
  status: "invited",
  is_agent: false,
  roles: ["rep"],
  team_ids: ["t-nordics"],
  created_at: "2026-09-30T09:00:00Z",
  allowed_actions: EVERY_VERB,
};

const RETIRED: User = {
  id: "u-3",
  email: "otto@brandt.example",
  display_name: "Otto Fischer",
  timezone: "Europe/Berlin",
  status: "deactivated",
  is_agent: false,
  roles: ["rep"],
  team_ids: [],
  last_active_at: ago(24 * 60),
  created_at: "2026-01-20T09:00:00Z",
  allowed_actions: ["change_role", "reactivate"],
};

const HELD: User = {
  id: "u-5",
  email: "sam@brandt.example",
  display_name: "Sam Okafor",
  timezone: "Europe/Berlin",
  status: "suspended",
  is_agent: false,
  roles: ["manager"],
  team_ids: ["t-key"],
  last_active_at: ago(200),
  created_at: "2026-03-01T09:00:00Z",
  allowed_actions: ["change_role", "deactivate"],
};

// The agent seat owns records, so it is listed, and holds no role.
const AGENT: User = {
  id: "u-agent",
  email: "agent@brandt.gradion.local",
  display_name: "Brandt Agent",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: true,
  roles: [],
  created_at: "2026-01-12T09:00:00Z",
  allowed_actions: ["deactivate"],
};

const LONG: User = {
  id: "u-long",
  email:
    "maximiliane.von-hohenzollern-sigmaringen@vertrieb.brandt-industrieanlagen.example",
  display_name: "Maximiliane Theodora von Hohenzollern-Sigmaringen",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: false,
  roles: ["manager"],
  team_ids: TEAMS.map((team) => team.id),
  last_active_at: ago(0.2),
  created_at: "2026-04-01T09:00:00Z",
  allowed_actions: EVERY_VERB,
};

const ROSTER = [LARS, DANA, IVY, HELD, RETIRED, AGENT];
const ADMIN: { roles: string[]; allow: GrantSpec } = {
  roles: ["admin"],
  allow: { user_admin: ["read", "create", "update", "delete"] },
};

type Users = () => Response | Promise<Response>;

function story(
  users: Users,
  identity: { roles: string[]; allow?: GrantSpec } = ADMIN,
  passwordLinks = false,
) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({ roles: identity.roles, allow: identity.allow }),
          admin_password_link: passwordLinks,
        }),
      "GET /users": users,
      "GET /teams": () => jsonResponse({ data: TEAMS, page: {} }),
      "GET /users/assignable-roles": () =>
        jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES }),
    });
    return (
      <StoryProviders>
        <UsersAdminCard />
      </StoryProviders>
    );
  };
}

const roster = (data: Partial<User>[]) => () =>
  jsonResponse({ data, page: {} });

const meta: Meta<typeof UsersAdminCard> = {
  title: "Settings/People/Members/Members",
  component: UsersAdminCard,
};
export default meta;
type Story = StoryObj<typeof UsersAdminCard>;

// Every status a seat can be in, the agent seat, and Lars as the one admin:
// his picker is the same control, refused with the reason beside it.
export const Roster: Story = { render: story(roster(ROSTER), ADMIN, true) };

// Under 36rem the table folds: member with its status and the menu on line
// one; role, teams and activity on line two.
export const RosterPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(roster(ROSTER), ADMIN, true),
};

export const LongContent: Story = {
  render: story(roster([LARS, LONG, DANA]), ADMIN, true),
};

export const OnlyTheAdmin: Story = { render: story(roster([LARS])) };

export const Empty: Story = { render: story(roster([])) };

export const Loading: Story = {
  render: story(() => new Promise<Response>(() => undefined)),
};

export const Failed: Story = {
  render: story(() =>
    jsonResponse(
      { title: "Internal Server Error", status: 500, detail: "Read failed." },
      500,
    ),
  ),
};

// A delegated administrator: last activity and the role change are withheld on
// members they do not outrank, and read as not available and outside their access.
export const Delegated: Story = {
  render: story(
    roster(
      ROSTER.map((member) =>
        (member.roles ?? []).includes("admin")
          ? { ...member, last_active_at: undefined, allowed_actions: [] }
          : member,
      ),
    ),
    {
      roles: ["custom"],
      allow: { user_admin: ["read", "create", "update", "delete"] },
    },
  ),
};

// The roster read without `user_admin:read`: no roles, teams, activity or
// verbs, and the card says once that managing users is not this reader's.
export const NotAnAdmin: Story = {
  render: story(
    roster(
      ROSTER.filter((member) => member.status !== "deactivated").map(
        ({
          roles: _roles,
          allowed_actions: _actions,
          team_ids: _teams,
          last_active_at: _activity,
          ...rest
        }) => rest,
      ),
    ),
    { roles: ["ops"] },
  ),
};

export const InviteDialog: Story = {
  render: story(roster([LARS, DANA]), {
    roles: ["admin"],
    allow: { user_admin: ["read", "create"] },
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Invite user" }),
    );
    await within(document.body).findByRole("dialog");
  },
};

export const RowMenuOpen: Story = {
  render: story(roster(ROSTER), ADMIN, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Dana Kessler" }),
    );
  },
};
