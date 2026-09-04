// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { StageLadder } from "./stageladder";

// The ladder every record with stages draws: a deal's pipeline, a project's
// phases, a lead's rungs. The stories below are the states that differ, which
// is what the component's props are for.
const meta: Meta<typeof StageLadder> = {
  title: "Design System/StageLadder",
  component: StageLadder,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof StageLadder>;

const noop = () => undefined;

/**
 * A deal mid-pipeline. Two stages behind it carry the trail, Negotiation is
 * the marker, and the two ways out stand off from the run of open stages.
 */
export const MidPipeline: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      steps={[
        { key: "q", label: "Qualified", done: true, onPick: noop },
        { key: "p", label: "Proposal", done: true, onPick: noop },
        { key: "n", label: "Negotiation", current: true, onPick: noop },
        { key: "w", label: "Won", terminal: true, onPick: noop },
        { key: "l", label: "Lost", terminal: true, onPick: noop },
      ]}
    />
  ),
};

/**
 * The first rung, with nothing behind it — the state a record spends its first
 * day in, and the one where a trail would be a lie.
 */
export const AtTheStart: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      steps={[
        { key: "q", label: "Qualified", current: true, onPick: noop },
        { key: "p", label: "Proposal", onPick: noop },
        { key: "n", label: "Negotiation", onPick: noop },
        { key: "w", label: "Won", terminal: true, onPick: noop },
        { key: "l", label: "Lost", terminal: true, onPick: noop },
      ]}
    />
  ),
};

/**
 * Every move refused for one cause — an archived record, a mirror the
 * incumbent will not let us write. The sentence belongs to the ladder rather
 * than to each step, so the page says it once.
 */
export const NoMoveAllowed: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      steps={[
        { key: "q", label: "Qualified", done: true, onPick: noop },
        {
          key: "p",
          label: "Proposal",
          current: true,
          onPick: noop,
        },
        {
          key: "n",
          label: "Negotiation",
          reason: "Restore this deal before moving it.",
          onPick: noop,
        },
        {
          key: "w",
          label: "Won",
          terminal: true,
          reason: "Restore this deal before moving it.",
          onPick: noop,
        },
        {
          key: "l",
          label: "Lost",
          terminal: true,
          reason: "Restore this deal before moving it.",
          onPick: noop,
        },
      ]}
    />
  ),
};

/**
 * With the line underneath: how the record got where it is. The lead has said
 * this since before the ladder was shared; the deal and the project can now.
 */
export const WithAHint: Story = {
  render: () => (
    <StageLadder
      label="Status"
      steps={[
        { key: "new", label: "New", done: true, onPick: noop },
        { key: "contacted", label: "Contacted", current: true, onPick: noop },
        { key: "engaged", label: "Engaged", onPick: noop },
        { key: "promoted", label: "Qualified", terminal: true, onPick: noop },
        {
          key: "disqualified",
          label: "Disqualified",
          terminal: true,
          onPick: noop,
        },
      ]}
      hint="Moved by Margince from a captured reply on 2 June."
    />
  ),
};

/**
 * A record whose stage the pipeline cannot name — an overlay mirror carrying
 * the incumbent's stage id. No marker and no trail, because both would be
 * guesses about where it has been.
 */
export const StageNotInThisPipeline: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      steps={[
        { key: "q", label: "Qualified", onPick: noop },
        { key: "p", label: "Proposal", onPick: noop },
        { key: "n", label: "Negotiation", onPick: noop },
        { key: "w", label: "Won", terminal: true, onPick: noop },
        { key: "l", label: "Lost", terminal: true, onPick: noop },
      ]}
    />
  ),
};
