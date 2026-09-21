// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { ExtensionIngestHealthCard } from "./extingesthealth";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Whether an installed connector is sending records this CRM cannot represent.
// The card stands beside the job-health reading on the same page and answers the
// same operational question, so the three states worth looking at are the three
// it can honestly be in: something was refused, nothing was, and the reader may
// not ask.

const meta: Meta<typeof ExtensionIngestHealthCard> = {
  title: "Settings/Governance/System health/Connector records refused",
  component: ExtensionIngestHealthCard,
};
export default meta;
type Story = StoryObj<typeof ExtensionIngestHealthCard>;

// Every story serves /me too: the card gates its own fetch on the grant, so a
// story without a principal would only ever render the withheld state.
function story(health: Record<string, unknown>, roles: string[] = ["admin"]) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ roles, allow: { job_health: ["read"] } })),
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
