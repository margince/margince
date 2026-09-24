// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { StoryProviders } from "../screens/story-utils";
import { Button } from "./atoms";
import { stubClipboard } from "./clipboard-testing";
import { useClipboardCopy } from "./clipboardcopy";

// The three states one copy control has, drawn by the hook that owns all three.
//
// Worth a story even though a hook draws nothing itself: the states are what a
// caller has to wire, and the reason this exists is that eight screens wired
// them eight ways. What a reader should take from this page is the SHAPE — a
// verb whose word changes, and a notice that appears under it saying what to do
// instead, never a button that quietly did nothing.

const LINK = "https://crm.example.test/#/rooms/acme-expansion?k=one-time";

function CopyTheLink({ text = LINK }: Readonly<{ text?: string }>) {
  const copy = useClipboardCopy(text, {
    copy: "Copy link",
    copied: "Copied",
    remedy: "Select the link and copy it manually.",
  });
  return (
    <div style={{ display: "grid", gap: "var(--space-3)", maxWidth: "32rem" }}>
      <code>{text}</code>
      <div>
        <Button onClick={copy.copy}>{copy.label}</Button>
      </div>
      {copy.notice}
    </div>
  );
}

const meta: Meta<typeof CopyTheLink> = {
  title: "Design System/useClipboardCopy",
  component: CopyTheLink,
  parameters: { layout: "padded" },
  render: () => (
    <StoryProviders>
      <CopyTheLink />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof CopyTheLink>;

/** Nothing has happened yet: the verb, and no notice. */
export const Idle: Story = {};

/** The copy landed. The word changes and nothing else moves — a notice here
 *  would be the surface congratulating itself. */
export const Copied: Story = {
  play: async ({ canvasElement }) => {
    const clipboard = stubClipboard("accepts");
    try {
      await userEvent.click(
        within(canvasElement).getByRole("button", { name: "Copy link" }),
      );
      await within(canvasElement).findByRole("button", { name: "Copied" });
    } finally {
      clipboard.restore();
    }
  },
};

/**
 * The browser would not take it. `navigator.clipboard` is UNDEFINED outside a
 * secure context, so the play takes it away rather than making a write throw —
 * that is the shape the real failure has, and it is the state the eight
 * hand-rolled sites each answered differently.
 */
export const Refused: Story = {
  play: async ({ canvasElement }) => {
    // Put back whatever this browser had: the catalog renders many stories on
    // one page, and a clipboard taken away for good would make the next surface
    // that copies fail for a reason nobody could find here.
    const clipboard = stubClipboard("absent");
    try {
      await userEvent.click(
        within(canvasElement).getByRole("button", { name: "Copy link" }),
      );
      await screen.findByText("Clipboard access denied");
    } finally {
      clipboard.restore();
    }
  },
};
