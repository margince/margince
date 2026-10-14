// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { TextInput } from "./atoms";
import { Panel } from "./panel";
import { PanelNotices } from "./panelnotices";
import { PanelSave } from "./panelsave";
import { SettingList, SettingRow } from "./settingrow";

// The save rule for a card with several fields: Discard and Save at its foot,
// only while it differs from what was saved. A lone Switch writes on flip.
const meta: Meta<typeof PanelSave> = {
  title: "Components/Forms and input/Panel save",
  component: PanelSave,
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
type Story = StoryObj<typeof PanelSave>;

type Names = Readonly<{ display: string; greeting: string }>;
const SAVED: Names = { display: "Marek Janetzke", greeting: "Marek" };

function NamesCard({
  initial = SAVED,
  pending = false,
  refused,
}: Readonly<{
  initial?: Names;
  pending?: boolean;
  refused?: Readonly<{ title: string; error: unknown }>;
}>) {
  const [saved, setSaved] = useState(SAVED);
  const [draft, setDraft] = useState(initial);
  const dirty =
    draft.display !== saved.display || draft.greeting !== saved.greeting;
  return (
    <Panel
      title="Names"
      actions={
        <PanelSave
          dirty={dirty}
          pending={pending}
          caption="Unsaved changes"
          onSave={() => setSaved(draft)}
          onDiscard={() => setDraft(saved)}
        />
      }
    >
      <SettingList bleed="settings">
        <SettingRow
          label="Display name"
          description="How colleagues see you on records and mail."
          control={(control) => (
            <TextInput
              {...control}
              className="settingrow-measure"
              value={draft.display}
              onChange={(e) => setDraft({ ...draft, display: e.target.value })}
            />
          )}
        />
        <SettingRow
          label="Greeting name"
          description="What Margince calls you."
          control={(control) => (
            <TextInput
              {...control}
              className="settingrow-measure"
              value={draft.greeting}
              onChange={(e) => setDraft({ ...draft, greeting: e.target.value })}
            />
          )}
        />
      </SettingList>
      <PanelNotices refused={refused} />
    </Panel>
  );
}

const EDITED: Names = { display: "Marek J.", greeting: "Marek" };

export const Clean: Story = {
  render: () => <NamesCard />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("heading", { name: "Names" });
    await expect(canvas.queryByRole("button")).toBeNull();
    await expect(
      canvasElement.querySelector(".panel-actions"),
    ).not.toBeVisible();
  },
};

// Typing makes the card dirty; Discard puts the saved values back.
export const Dirty: Story = {
  render: () => <NamesCard initial={EDITED} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Unsaved changes")).toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "Discard" }));
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

export const Pending: Story = {
  render: () => <NamesCard initial={EDITED} pending />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const save = await canvas.findByRole("button", { name: "Save" });
    await expect(save).toHaveAttribute("aria-disabled", "true");
  },
};

// A refused save keeps the draft and says why once, for the whole card.
export const RefusedWithPanelNotices: Story = {
  render: () => (
    <NamesCard
      initial={EDITED}
      refused={{ title: "Not saved", error: new Error("network down") }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Not saved")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Save" })).toBeVisible();
  },
};

export const Phone: Story = {
  render: () => (
    <div style={{ padding: "var(--padCard)" }}>
      <NamesCard initial={EDITED} />
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
