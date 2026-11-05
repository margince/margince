// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { StoryProviders } from "../../testing/story-utils";
import { AudienceMembers } from "./audiencemembers";

// The checklist behind the `selected` audience, shown in the two states it
// actually has: a roster to tick, and a roster that has not arrived. It takes
// its candidates and its ticks as props and asks the network for nothing, so
// these stories hand it the lists the timeline dialog and the message-access
// drawer would have read.

type Props = ComponentProps<typeof AudienceMembers>;

// Seats carry their address as the note — two colleagues share a first name
// often enough that the name alone is a coin flip — and a team carries none.
const LIST: Props["candidates"]["list"] = [
  { kind: "user", id: "u-1", name: "Anna Weber", note: "anna@margince.test" },
  { kind: "user", id: "u-2", name: "Jonas Weber", note: "jonas@margince.test" },
  { kind: "user", id: "u-3", name: "Priya Raman", note: "priya@margince.test" },
  { kind: "team", id: "t-1", name: "Deal desk" },
];

// The two roster reads behind the list, as each story has them.
function rosterRead(isError: boolean): Props["candidates"]["users"] {
  return { isError, refetch: () => undefined };
}

function candidates(
  list: Props["candidates"]["list"],
  failed = false,
): Props["candidates"] {
  return { list, users: rosterRead(failed), teams: rosterRead(false) };
}

const CHOSEN: Props["chosen"] = [
  { subject_type: "user", subject_id: "u-2" },
  { subject_type: "team", subject_id: "t-1" },
];

const meta: Meta<typeof AudienceMembers> = {
  title: "Patterns/Audience members",
  component: AudienceMembers,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    candidates: candidates(LIST),
    chosen: CHOSEN,
    onChange: () => {},
  },
};
export default meta;

type Story = StoryObj<typeof meta>;

/** A seat and a team ticked: what the message will be limited to, in full. */
export const SomebodyNamed: Story = {};

/**
 * Nothing ticked yet. The set it would submit is empty, which is why the
 * editors above it keep Save back until somebody is named.
 */
export const NobodyNamedYet: Story = { args: { chosen: [] } };

/**
 * The roster has not answered. Told rather than shown an empty box, which
 * would read as a workspace with nobody in it.
 */
export const RosterNotRead: Story = { args: { candidates: candidates([]) } };

/** The colleague read failed: said so, with Retry, beside the teams it has. */
export const ColleaguesFailed: Story = {
  args: { candidates: candidates(LIST.slice(3), true) },
};

// The ticked rows carry a checkbox accent and the notes sit at caption
// contrast: both are derived tokens, so the dark ground is where they can
// drift apart.
export const SomebodyNamedDark: Story = { globals: { theme: "dark" } };
