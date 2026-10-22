// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Button, Field, Textarea } from "./atoms";
import { DockedWindow, type DockState } from "./dockedwindow";

const meta: Meta<typeof DockedWindow> = {
  title: "Components/Overlays and layering/Docked window",
  component: DockedWindow,
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

function Demo({ start }: Readonly<{ start: DockState }>) {
  const [state, setState] = useState<DockState>(start);
  const [open, setOpen] = useState(true);
  return (
    <>
      <p className="t-body">
        The record behind the window stays in reach: this text can still be
        selected while the window is open.
      </p>
      <Button onClick={() => setOpen(true)}>Write email</Button>
      {open && (
        <DockedWindow
          title="Re: Budget questions"
          state={state}
          onStateChange={setState}
          onClose={() => setOpen(false)}
        >
          <div style={{ padding: "var(--space-4)" }}>
            <Field label="Message">
              {(control) => <Textarea {...control} rows={8} />}
            </Field>
          </div>
        </DockedWindow>
      )}
    </>
  );
}

/** Docked at its own size in the bottom end corner, the page usable behind. */
export const Docked: StoryObj = {
  render: () => <Demo start="open" />,
  play: async () => {
    await expect(
      await screen.findByRole("dialog", { name: "Re: Budget questions" }),
    ).toHaveAttribute("aria-modal", "false");
  },
};

/** Folded to its title bar, which opens it again. */
export const Minimized: StoryObj = {
  render: () => <Demo start="min" />,
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Open Re: Budget questions" }),
    );
    await screen.findByRole("dialog", { name: "Re: Budget questions" });
  },
};

/** Grown: wider, and as tall as the glass allows. */
export const Grown: StoryObj = { render: () => <Demo start="max" /> };

export const DockedDark: StoryObj = {
  ...Docked,
  globals: { theme: "dark" },
};

export const DockedOnPhone: StoryObj = {
  ...Docked,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
