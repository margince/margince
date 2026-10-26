// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { StageLadder } from "./stageladder";

// The ladder every record with stages draws: a deal's pipeline, a project's
// phases, a lead's rungs. The stories below are the states that differ, which
// is what the component's props are for.
const meta: Meta<typeof StageLadder> = {
  title: "Components/Forms and input/Stage ladder",
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
 * Every move refused for one cause — an archived record, say. The sentence
 * belongs to the ladder rather than to each step, so the page says it once.
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
 * A record whose stage the pipeline cannot name — one archived out from under
 * it. No marker and no trail, because both would be guesses about where it has
 * been.
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

/**
 * A pipeline a workspace built for itself, longer than any row has room for.
 * The deal stands two-thirds of the way along it, so the stages it has
 * climbed, where it is and the ways out cannot all sit in one row at once.
 */
export const LongPipeline: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      steps={[
        { key: "in", label: "Inbound", done: true, onPick: noop },
        { key: "disc", label: "Discovery", done: true, onPick: noop },
        { key: "q", label: "Qualified", done: true, onPick: noop },
        { key: "demo", label: "Demo scheduled", done: true, onPick: noop },
        { key: "tech", label: "Technical review", done: true, onPick: noop },
        { key: "p", label: "Proposal sent", done: true, onPick: noop },
        { key: "n", label: "Negotiation", current: true, onPick: noop },
        { key: "legal", label: "Legal review", onPick: noop },
        { key: "proc", label: "Procurement", onPick: noop },
        { key: "sig", label: "Awaiting signature", onPick: noop },
        { key: "w", label: "Won", terminal: true, onPick: noop },
        { key: "l", label: "Lost", terminal: true, onPick: noop },
      ]}
    />
  ),
};

// Nine open stages of two long words each, the deal standing on the fourth.
const NINE_STAGES = [
  "Potential Deal",
  "Deal Qualification",
  "Solution Discovery",
  "Conversion Strategy",
  "Proposal Sent",
  "Contract Negotiation",
  "Contract Review",
  "Legal Approval",
  "Final Signature",
];

function nineStages(labels: readonly string[]) {
  const here = 3;
  return [
    ...labels.map((label, index) => ({
      key: label,
      label,
      done: index < here,
      current: index === here,
      onPick: noop,
    })),
    { key: "won", label: "Closed Won", terminal: true, onPick: noop },
    { key: "lost", label: "Closed Lost", terminal: true, onPick: noop },
  ];
}

// A button stands at a clipped end and at no other. Measured, not assumed:
// whether nine names fit depends on the font the frame renders.
async function pagersMatchOverflow(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const run = canvasElement.querySelector<HTMLElement>(".stage-ladder-run");
  if (!run) {
    throw new Error("The ladder drew no run of open stages.");
  }
  const later = () =>
    canvas.queryByRole("button", { name: "Show later stages" });
  const earlier = () =>
    canvas.queryByRole("button", { name: "Show earlier stages" });
  await waitFor(() => {
    const hidden = run.scrollWidth - run.clientWidth;
    expect(Boolean(earlier())).toBe(hidden > 1 && run.scrollLeft > 1);
    expect(Boolean(later())).toBe(hidden > 1 && run.scrollLeft < hidden - 1);
  });
  return { run, later, earlier };
}

async function pageToTheEnd(canvasElement: HTMLElement) {
  const { run, later, earlier } = await pagersMatchOverflow(canvasElement);
  if (run.scrollWidth - run.clientWidth <= 1) {
    return;
  }
  const user = userEvent.setup();
  for (let press = 0; press < 12 && later(); press += 1) {
    const button = later();
    if (button) {
      await user.click(button);
    }
    await pagersMatchOverflow(canvasElement);
  }
  await waitFor(() => expect(later()).toBeNull());
  expect(earlier()).not.toBeNull();
}

/**
 * Nine long stage names in a 1280px frame. The run cannot hold them, so it
 * rests on the current stage with a fade and a button at each clipped end.
 * Paging to the end takes the later button away and keeps the earlier one.
 */
export const NineStagesAt1280: Story = {
  render: () => (
    <div style={{ inlineSize: 1280 }}>
      <StageLadder label="Stage" steps={nineStages(NINE_STAGES)} />
    </div>
  ),
  play: async ({ canvasElement }) => pageToTheEnd(canvasElement),
};

/** The same pipeline in a 1500px frame, the width the clipping was seen at. */
export const NineStagesAt1500: Story = {
  render: () => (
    <div style={{ inlineSize: 1500 }}>
      <StageLadder label="Stage" steps={nineStages(NINE_STAGES)} />
    </div>
  ),
  play: async ({ canvasElement }) => pageToTheEnd(canvasElement),
};

/** Nine short names that fit a 1500px frame: no fade and no button. */
export const NineStagesThatFit: Story = {
  render: () => (
    <div style={{ inlineSize: 1500 }}>
      <StageLadder
        label="Stage"
        steps={nineStages([
          "One",
          "Two",
          "Three",
          "Four",
          "Five",
          "Six",
          "Seven",
          "Eight",
          "Nine",
        ])}
      />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const { run, later, earlier } = await pagersMatchOverflow(canvasElement);
    expect(run.scrollWidth - run.clientWidth).toBeLessThanOrEqual(1);
    expect(later()).toBeNull();
    expect(earlier()).toBeNull();
  },
};

/**
 * The bars shape inside How it stands: a deal drifting at its third of seven
 * stages, each stage with the days it held. The current bar takes the
 * verdict's warning tone, and "Move to" names the stage after it.
 */
export const BarsMidPipeline: Story = {
  render: () => (
    <StageLadder
      label="Stage"
      variant="bars"
      tone="warning"
      steps={[
        {
          key: "pd",
          label: "Potential Deal",
          done: true,
          sub: "12 d",
          onPick: noop,
        },
        {
          key: "dq",
          label: "Deal Qualification",
          done: true,
          sub: "9 d",
          onPick: noop,
        },
        {
          key: "sd",
          label: "Solution Discovery",
          current: true,
          sub: "6 d · now",
          onPick: noop,
        },
        { key: "cs", label: "Conversion Strategy", onPick: noop },
        { key: "ps", label: "Proposal Sent", onPick: noop },
        { key: "ng", label: "Negotiation", onPick: noop },
        { key: "cr", label: "Contract Review", onPick: noop },
        { key: "won", label: "Closed Won", terminal: true, onPick: noop },
        { key: "lost", label: "Closed Lost", terminal: true, onPick: noop },
      ]}
    />
  ),
};

/**
 * A lead at its last open rung: no "Move to", and the two ways out offered as
 * dashed pills.
 */
export const BarsLastRung: Story = {
  render: () => (
    <StageLadder
      label="Status"
      variant="bars"
      steps={[
        { key: "new", label: "New", done: true, sub: "2 Aug", onPick: noop },
        {
          key: "contacted",
          label: "Contacted",
          done: true,
          sub: "4 Aug",
          onPick: noop,
        },
        {
          key: "engaged",
          label: "Engaged",
          current: true,
          sub: "9 Aug · now",
          onPick: noop,
        },
        { key: "promoted", label: "Qualified", terminal: true, onPick: noop },
        {
          key: "disqualified",
          label: "Disqualified",
          terminal: true,
          onPick: noop,
        },
      ]}
    />
  ),
};

/**
 * A closed project: the way out it took is a chip, and no open stage is
 * current, so the summary names no position. No dates were recorded, so no
 * stage carries a sub line.
 */
export const BarsClosedWithoutDates: Story = {
  render: () => (
    <StageLadder
      label="Phase"
      variant="bars"
      tone="neutral"
      steps={[
        { key: "initiative", label: "Initiative", done: true, onPick: noop },
        { key: "pursuing", label: "Pursuing", done: true, onPick: noop },
        { key: "delivering", label: "Delivering", done: true, onPick: noop },
        {
          key: "closed",
          label: "Closed",
          terminal: true,
          current: true,
          onPick: noop,
        },
      ]}
    />
  ),
};
