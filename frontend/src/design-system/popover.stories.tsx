// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Popover } from "./popover";

// Popover: a short aside opened beside its trigger, portalled to the body so a
// card's own overflow clip cannot cut it off. The cases on this canvas are the
// ones the component itself distinguishes — click vs. the settled-hover open,
// a trigger drawn as a control vs. one that reads as text in its line — plus
// the one the CSS caps rather than lets run the page long: a panel of more
// rows than the viewport holds.
const meta: Meta<typeof Popover> = {
  title: "Design System/Popover",
  component: Popover,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Popover>;

async function openByClick({ canvasElement }: { canvasElement: HTMLElement }) {
  const canvas = within(canvasElement);
  await userEvent.click(canvas.getByRole("button", { name: "Evidence" }));
}

// A text trigger, no `variant`: the popover-trigger CSS strips the button
// chrome so it reads as words in the line it sits in rather than a control.
export const ClickOpenedTextTrigger: Story = {
  render: () => (
    <p>
      Renewal likely closes this quarter.{" "}
      <Popover label="Evidence">
        <p>Three emails and one call reference the March renewal date.</p>
      </Popover>
    </p>
  ),
  play: openByClick,
};

// A `variant`, so the trigger is a full `Button` — the caret half of a split
// control, or a toolbar verb — rather than text.
export const ClickOpenedButtonTrigger: Story = {
  render: () => (
    <Popover label="Evidence" variant="ghost">
      <p>Three emails and one call reference the March renewal date.</p>
    </Popover>
  ),
  play: openByClick,
};

// `onHover`: opens once the pointer has SETTLED on the trigger rather than on
// contact, so this waits for the panel rather than asserting it is open the
// instant the pointer arrives.
export const HoverOpened: Story = {
  render: () => (
    <p>
      Renewal likely closes this quarter.{" "}
      <Popover label="Evidence" onHover>
        <p>Three emails and one call reference the March renewal date.</p>
      </Popover>
    </p>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.hover(canvas.getByRole("button", { name: "Evidence" }));
    await waitFor(() =>
      expect(canvas.getByText(/Three emails and one call/)).toBeVisible(),
    );
  },
};

// A panel long enough that the fixed `max-inline-size` and `overflow-y: auto`
// in popover.css are the only thing keeping it inside the viewport rather than
// running the page long underneath it.
// A panel long enough to reach its own cap and scroll. Written out rather than
// counted from an index: a figure rendered into a story is a figure drawn in
// nobody's notation, and the list only has to be LONG.
const TOUCHPOINTS = Array.from(
  { length: 24 },
  (_, index) => `Touchpoint ${"i".repeat(index + 1)} — reply logged`,
);

export const LongPanelScrolls: Story = {
  render: () => (
    <Popover label="Evidence" variant="ghost">
      <ul>
        {TOUCHPOINTS.map((touchpoint) => (
          <li key={touchpoint}>{touchpoint}</li>
        ))}
      </ul>
    </Popover>
  ),
  play: openByClick,
};
