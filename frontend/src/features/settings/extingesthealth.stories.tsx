// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ExtensionIngestHealthCard } from "./extingesthealth";

// Whether an installed connector is sending records this CRM cannot represent.
// The card stands beside the job-health reading on the same page and answers the
// same operational question, so the three states worth looking at are the three
// it can honestly be in: something was refused, nothing was, and the reader may
// not ask.

const meta = {
  title: "Settings/Governance/System health/Connector records refused",
  component: ExtensionIngestHealthCard,
} satisfies Meta<typeof ExtensionIngestHealthCard>;
export default meta;
type Story = StoryObj<typeof meta>;

// The seeded matrix gives `job_health:read` to admin and ops only, so a story's
// grant follows its roles and Withheld really draws the withheld state.
const READS_JOB_HEALTH = ["admin", "ops"];

function grantFor(roles: string[]): GrantSpec {
  return roles.some((role) => READS_JOB_HEALTH.includes(role))
    ? { job_health: ["read"] }
    : {};
}

// Every story serves /me too: the card gates its own fetch on the grant, so a
// story without a principal would only ever render the withheld state.
function story(health: Record<string, unknown>, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: grantFor(roles) })),
      "GET /admin/extension-ingest-health": () => jsonResponse(health),
    });
    return (
      <StoryProviders>
        <ExtensionIngestHealthCard />
      </StoryProviders>
    );
  };
}

const REFUSING = {
  generated_at: "2026-08-13T09:30:00Z",
  window_days: 7,
  units: [
    {
      unit: "acme-mailbridge",
      refused: 34,
      last_refused_at: "2026-08-13T03:14:00Z",
      refusals: [
        { refusal: "counterparty", refused: 21 },
        { refusal: "addresses", refused: 13 },
      ],
    },
    {
      unit: "nordwind-telematics",
      refused: 2,
      last_refused_at: "2026-08-11T18:02:00Z",
      refusals: [{ refusal: "size", refused: 2 }],
    },
  ],
};

// Two connectors refusing for different reasons. What to look at is the warning
// pills: they qualify the unit's own name beside them, so the row has to read as
// "this connector, these refusals" rather than as a strip of statuses.
export const Refusing: Story = { render: story(REFUSING) };

// The answer an operator opens the card for, and it is a finding rather than a
// gap: every record the window covers was representable. The sentence names the
// window, because "nothing here" would read as a card that failed to answer.
export const NothingRefused: Story = {
  render: story({
    generated_at: "2026-08-13T09:30:00Z",
    window_days: 7,
    units: [],
  }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText(/No records refused in the last 7 days/);
    await expect(
      canvas.queryByText(/Shows counts and the refusing/),
    ).toBeNull();
  },
};

// A reader without the grant. The card keeps its place and says the reading is
// not theirs — an absent card would read as "this installation refuses nothing".
export const Withheld: Story = { render: story(REFUSING, ["rep"]) };

// The same refusals in dark, where the warning tint and the card ground are
// both re-derived: a pill that reads as a status in light can stop reading as a
// pill at all against the darker plate.
export const RefusingDark: Story = {
  globals: { theme: "dark" },
  render: story(REFUSING),
};

export const RefusingAtPhoneWidth: Story = {
  ...Refusing,
  tags: ["uat-phone"],
};
