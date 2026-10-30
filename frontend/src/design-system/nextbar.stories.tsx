// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { NextBar } from "./nextbar";

const meta: Meta<typeof NextBar> = {
  title: "Components/Layout and structure/Next bar",
  component: NextBar,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "56rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof NextBar>;

const noop = () => undefined;

/** An overdue promise: the late tag, why it is next, the verb, Snooze, and
 *  the other open tasks. */
export const LatePromise: Story = {
  args: {
    title: "Send Dana the renewal paperwork",
    late: { label: "19 days late", tone: "danger" },
    sub: "Due 25 Jul · promised after the 18 Jun proposal",
    action: (
      <Button variant="primary" onClick={noop}>
        Send paperwork
      </Button>
    ),
    snooze: { onSnooze: noop },
    more: { count: 2, onOpen: noop },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByRole("button", { name: "Snooze" })).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "+2 tasks" }),
    ).toBeVisible();
  },
};

/** A lead ready to qualify: a good tag and an outlined verb, nothing to
 *  snooze. */
export const ReadyToQualify: Story = {
  args: {
    title: "Ready to qualify",
    late: { label: "Demo booked", tone: "success" },
    sub: "Jonas replied and booked a demo, which is the evidence qualifying asks for.",
    action: (
      <Button variant="ghost" onClick={noop}>
        Qualify and open deal
      </Button>
    ),
    more: { count: 1, onOpen: noop },
  },
};

/** A task with its own words under the title, cut at one line on a narrow
 *  bar, with a German label that runs long. */
export const TaskWithDetail: Story = {
  args: {
    title: "Rückruf wegen der Verlängerungsvereinbarung",
    detail:
      "Zweiter Versuch: Dana fragt nach den Preisstufen für die Verlängerung und möchte vor Freitag eine Zusage.",
    sub: "Fällig am 25. Juli",
    more: { count: 1, onOpen: noop },
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "20rem" }}>
        <Story />
      </div>
    ),
  ],
  play: async ({ canvasElement }) => {
    const detail = within(canvasElement).getByText(/^Zweiter Versuch/);
    await expect(detail).toBeVisible();
    await expect(detail.scrollWidth).toBeGreaterThan(detail.clientWidth);
  },
};

/** Nothing dated next: one quiet sentence rather than an empty bar. */
export const NothingNext: Story = {
  args: {},
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("Nothing is due next."),
    ).toBeVisible();
  },
};

/** "+2 tasks" names the two on hover and focus; a row opens the list. */
export const MoreTasksNamed: Story = {
  args: {
    title: "Send the revised retrofit quote",
    sub: "Due 17 Oct",
    more: {
      count: 2,
      onOpen: noop,
      peek: {
        head: "Other open tasks",
        items: [
          {
            key: "a",
            title: "Book the site visit",
            sub: "28 Oct",
            onOpen: noop,
          },
          {
            key: "b",
            title: "Update the capacity sheet",
            sub: "No date",
            onOpen: noop,
          },
        ],
      },
    },
  },
};

/** German in a narrow column: the title wraps, and the late tag and the verbs
 *  stay whole. */
export const LongGermanNarrow: Story = {
  decorators: [
    (Story) => (
      <LocaleProvider initial="de">
        <div lang="de" style={{ maxWidth: "26rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
  args: {
    title:
      "Dem Beschaffungsleiter die überarbeiteten Vertragsunterlagen senden",
    late: { label: "19 Tage überfällig", tone: "danger" },
    sub: "Fällig am 25. Juli · zugesagt nach dem Angebot vom 18. Juni",
    action: (
      <Button variant="primary" onClick={noop}>
        Unterlagen senden
      </Button>
    ),
    snooze: { onSnooze: noop },
    more: { count: 2, onOpen: noop },
  },
};

/** Three verbs and the task count on a phone: the verbs wrap inside the bar
 *  rather than run past its edge. */
export const ThreeVerbsOnPhone: Story = {
  args: {
    title: "You owe them: Call Dana back on Monday",
    sub: "Due in 2 days.",
    action: (
      <>
        <Button variant="primary" onClick={noop}>
          Write to them
        </Button>
        <Button onClick={noop}>Call</Button>
      </>
    ),
    snooze: { onSnooze: noop },
    more: { count: 3, onOpen: noop },
  },
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const bar = canvasElement.querySelector(".next-bar-row");
    const edge = bar?.getBoundingClientRect().right ?? 0;
    const verbs = canvasElement.querySelectorAll(".next-bar-verbs > *");
    await expect(verbs.length).toBe(4);
    for (const verb of verbs) {
      await expect(verb.getBoundingClientRect().right).toBeLessThanOrEqual(
        edge,
      );
    }
  },
};
