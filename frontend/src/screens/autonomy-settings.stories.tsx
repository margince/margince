// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutonomySettingsCard } from "./autonomy-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a rep has let answer itself. There is no permission story here: the card
// reads and writes the reader's own rows, so no seat can be shown the switches
// refused — which is why the fixtures below vary the RECORD instead.

type Row = {
  kind: string;
  mode: "manual" | "auto";
  approved_clean: number;
  approved_edited: number;
  rejected: number;
};

function row(kind: string, mode: "manual" | "auto", counts: number[]): Row {
  return {
    kind,
    mode,
    approved_clean: counts[0],
    approved_edited: counts[1],
    rejected: counts[2],
  };
}

function story(data: Row[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /autonomy": () => jsonResponse({ data }),
    });
    return (
      <StoryProviders>
        <AutonomySettingsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AutonomySettingsCard> = {
  title: "Settings/Account/What answers itself",
  component: AutonomySettingsCard,
};
export default meta;
type Story = StoryObj<typeof AutonomySettingsCard>;

// The first visit: three switches off, and nothing under any of them. A reader
// with no history is offered the choice on the description alone, so this story
// is the one that shows whether that description carries it.
export const NothingDecidedYet: Story = {
  render: story([
    row("close_date_correction", "manual", [0, 0, 0]),
    row("lifecycle_change", "manual", [0, 0, 0]),
    row("org_name_promotion", "manual", [0, 0, 0]),
  ]),
};

// The case the feature exists for: a rep who has approved fourteen close dates
// unchanged, and the record saying so under the switch they are about to move.
export const EarnedOnOne: Story = {
  render: story([
    row("close_date_correction", "auto", [14, 1, 0]),
    row("lifecycle_change", "manual", [2, 3, 4]),
    row("org_name_promotion", "manual", [6, 0, 1]),
  ]),
};

// A kind whose copy this catalog does not name, which is what a reader sees if
// the server starts offering a fourth before the strings land. The row is
// unpolished on purpose rather than hidden — a choice the reader now has is
// worth showing badly.
export const AKindTheCopyDoesNotKnow: Story = {
  render: story([
    row("close_date_correction", "auto", [14, 1, 0]),
    row("project_attribution", "manual", [0, 0, 0]),
  ]),
};

// Every row on, in dark. The switch's on-state is the card's only colour, so
// three of them together is where a wrong track token would show.
export const AllOnDark: Story = {
  globals: { theme: "dark" },
  render: story([
    row("close_date_correction", "auto", [14, 1, 0]),
    row("lifecycle_change", "auto", [9, 0, 0]),
    row("org_name_promotion", "auto", [21, 2, 1]),
  ]),
};
