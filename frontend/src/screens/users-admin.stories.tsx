// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { UsersAdminCard } from "./users-admin";

const LARS = {
  id: "u-1",
  email: "lars@brandt.example",
  display_name: "Lars Brandt",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: false,
  roles: ["admin"],
  team_ids: ["t-dach"],
  allowed_actions: ["issue_password_link"],
};

const DANA = {
  id: "u-2",
  email: "dana@brandt.example",
  display_name: "Dana Kessler",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: false,
  roles: ["rep"],
  team_ids: ["t-dach", "t-north"],
  allowed_actions: ["change_role", "issue_password_link", "deactivate"],
};

// A deactivated seat still occupies the roster: the card lists everyone with a
// place in the installation, which is the question it answers.
const RETIRED = {
  id: "u-3",
  email: "otto@brandt.example",
  display_name: "Otto Fischer",
  timezone: "Europe/Berlin",
  status: "deactivated",
  is_agent: false,
  roles: ["rep"],
  allowed_actions: ["change_role", "reactivate"],
};

// An invitation nobody has redeemed yet: it holds a seat, and the neutral badge
// says it is the ordinary first state rather than something wrong.
const INVITED = {
  id: "u-4",
  email: "ivy@brandt.example",
  display_name: "Ivy Lindqvist",
  timezone: "Europe/Berlin",
  status: "invited",
  is_agent: false,
  roles: ["manager"],
  team_ids: ["t-north"],
  allowed_actions: ["change_role", "issue_password_link", "deactivate"],
};

const TEAMS = [
  { id: "t-dach", name: "DACH Sales" },
  { id: "t-north", name: "Nordics" },
];

// An agent identity. Bootstrap seeds none, so most installations have no such
// row — but one exists wherever the retirement migration has not run, and a
// resident runner will land under the same flag. It owns records, so the roster
// lists it, and it has no role at all: the one row whose answer is a sentence
// rather than a picker. That state is what this story exists to show.
const AGENT = {
  id: "u-agent",
  email: "agent@brandt.gradion.local",
  display_name: "Brandt Agent",
  timezone: "Europe/Berlin",
  status: "active",
  is_agent: true,
  roles: [],
  allowed_actions: ["deactivate"],
};

// Nine members is the roster the founder measured: nine table rows, every role,
// status badge and menu trigger at one x.
const CROWD = [
  LARS,
  DANA,
  RETIRED,
  AGENT,
  ...["Mira Hoffmann", "Jonas Weber", "Ana Duarte", "Piet Klaassen"].map(
    (display_name, index) => ({
      id: `u-crowd-${index}`,
      email: `${display_name.split(" ")[0]?.toLowerCase()}@brandt.example`,
      display_name,
      timezone: "Europe/Berlin",
      status: "active",
      is_agent: false,
      roles: [index === 0 ? "manager" : "rep"],
      team_ids: index % 2 === 0 ? ["t-dach"] : [],
      allowed_actions: ["change_role", "issue_password_link", "deactivate"],
    }),
  ),
  {
    id: "u-crowd-unset",
    email: "new@brandt.example",
    display_name: "Sofia Marchetti",
    timezone: "Europe/Berlin",
    status: "active",
    is_agent: false,
    // No role yet, so the column says so rather than leaving a blank.
    roles: [],
    allowed_actions: ["change_role", "deactivate"],
  },
];

// `admin_password_link` is what decides whether a row's menu carries the
// set-password verb at all, and `meFixture` defaults it off — so a story about
// the row's verbs has to say so, or it shows a menu with one item.
function story(
  users: Record<string, unknown>[],
  identity: { roles?: string[]; seat?: "full" | "read" } = {},
  allow: GrantSpec = {},
  passwordLinks = false,
) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({ ...identity, allow }),
          admin_password_link: passwordLinks,
        }),
      "GET /users": () => jsonResponse({ data: users }),
      "GET /teams": () =>
        jsonResponse({ data: TEAMS, page: { has_more: false } }),
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

const meta: Meta<typeof UsersAdminCard> = {
  title: "Settings/People/Members/Members",
  component: UsersAdminCard,
};
export default meta;
type Story = StoryObj<typeof UsersAdminCard>;

const ROSTER = [LARS, DANA, INVITED, RETIRED, AGENT];

export const Roster: Story = {
  render: story(ROSTER, { roles: ["admin"] }, {}, true),
};

export const RosterDark: Story = {
  render: story(ROSTER, { roles: ["admin"] }, {}, true),
  globals: { theme: "dark" },
};

// Nine members read as nine rows down one set of columns.
export const NineMembers: Story = {
  render: story(CROWD, { roles: ["admin"] }, {}, true),
};

// The table is wider than a phone, so it scrolls inside its own region rather
// than squeezing the status badges; the toolbar stacks.
export const RosterPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ROSTER, { roles: ["admin"] }, {}, true),
};

export const Empty: Story = { render: story([], { roles: ["admin"] }) };

// The roster answers "who is on my team", which is not an admin's private
// question, but administering it is. An operator seat without the admin role
// reads the card with no role or team columns and no verbs, and the panel foot
// says that managing members is not theirs.
export const NotAnAdmin: Story = {
  render: story([LARS, DANA, RETIRED, AGENT], { roles: ["ops"] }),
};

export const NotAnAdminDark: Story = {
  render: story([LARS, DANA, RETIRED, AGENT], { roles: ["ops"] }),
  globals: { theme: "dark" },
};
