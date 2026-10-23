// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { TextInput } from "./atoms";
import { Panel } from "./panel";
import { PageSaveBar, SavePageProvider, useSaveDraft } from "./savepage";
import { SettingList, SettingRow } from "./settingrow";

// A settings page saves through one bar at its foot, drawn only while some card
// holds a draft. Each card hands its draft over with `useSaveDraft`.

const meta: Meta<typeof PageSaveBar> = {
  title: "Components/Forms and input/Page save bar",
  component: PageSaveBar,
};
export default meta;
type Story = StoryObj<typeof PageSaveBar>;

type Outcome = "saves" | "hangs" | "refuses";

function settle(outcome: Outcome): Promise<void> {
  if (outcome === "hangs") return new Promise(() => undefined);
  if (outcome === "refuses") {
    return Promise.reject(new Error("The server refused the change."));
  }
  return Promise.resolve();
}

function DraftCard({
  title,
  label,
  saved: initial,
  typed,
  outcome = "saves",
}: Readonly<{
  title: string;
  label: string;
  saved: string;
  typed?: string;
  outcome?: Outcome;
}>) {
  const [saved, setSaved] = useState(initial);
  const [draft, setDraft] = useState(typed ?? initial);
  useSaveDraft({
    dirty: draft !== saved,
    save: async () => {
      await settle(outcome);
      setSaved(draft);
    },
    discard: () => setDraft(saved),
  });
  return (
    <Panel title={title}>
      <SettingList bleed="settings">
        <SettingRow
          label={label}
          control={(control) => (
            <TextInput
              {...control}
              className="settingrow-measure"
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
            />
          )}
        />
      </SettingList>
    </Panel>
  );
}

function Page({
  typed,
  outcome,
  second,
}: Readonly<{ typed?: string; outcome?: Outcome; second?: string }>) {
  return (
    <SavePageProvider>
      <div className="settings-stack">
        <DraftCard
          title="Profile"
          label="Display name"
          saved="Jane Doe"
          typed={typed}
          outcome={outcome}
        />
        <DraftCard
          title="Signature"
          label="Job title"
          saved="Account Executive"
          typed={second}
        />
      </div>
      <PageSaveBar />
    </SavePageProvider>
  );
}

// Awaited, because a card registers its draft in an effect and the bar is
// drawn only once that has run. A synchronous read asks for the button in the
// paint before registration and finds a page with no bar on it.
const saveButton = (canvas: HTMLElement) =>
  within(canvas).findByRole("button", { name: "Save" });

/** Nothing is unsaved, so there is no bar at all. */
export const Clean: Story = {
  render: () => <Page />,
  play: async ({ canvasElement }) => {
    await expect(within(canvasElement).queryByRole("button")).toBeNull();
  },
};

export const Dirty: Story = {
  render: () => <Page typed="Jane Smith" />,
};
export const DirtyDark: Story = { ...Dirty, globals: { theme: "dark" } };

/** Two cards with drafts: the one Save commits both. */
export const TwoCardsDirty: Story = {
  render: () => <Page typed="Jane Smith" second="Head of Sales" />,
};

/** While the save runs, Save is busy and Discard is held. */
export const Saving: Story = {
  render: () => <Page typed="Jane Smith" outcome="hangs" />,
  play: async ({ canvasElement }) => {
    await userEvent.click(await saveButton(canvasElement));
    await expect(
      within(canvasElement).getByRole("button", { name: "Discard" }),
    ).toBeDisabled();
  },
};

/** A refused save keeps the bar and the draft, and says why once. */
export const Failed: Story = {
  render: () => <Page typed="Jane Smith" outcome="refuses" />,
  play: async ({ canvasElement }) => {
    await userEvent.click(await saveButton(canvasElement));
    await expect(await within(canvasElement).findByRole("alert")).toBeVisible();
  },
};

export const Phone: Story = {
  ...Dirty,
  globals: { viewport: { value: "phone" } },
};
