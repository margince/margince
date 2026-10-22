// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Button, TextInput } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import {
  PageSaveBar,
  SavePageProvider,
  useSaveDraft,
  useSavePage,
} from "../design-system/savepage";
import { LocaleProvider } from "../i18n";
import { UnsavedGuard, useUnsavedGuard } from "./unsaved";

// Leaving a settings page with a draft on it: the guard above the screen holds
// the page and asks Save, Discard or Stay. Without a page save bar it asks only
// Discard or Stay, which is what a lone editor's draft gets.

const meta: Meta = {
  title: "Shell/Leave with unsaved changes",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

function NameCard() {
  const [saved, setSaved] = useState("Jane Doe");
  const [draft, setDraft] = useState("Jane Smith");
  useSaveDraft({
    dirty: draft !== saved,
    save: async () => setSaved(draft),
    discard: () => setDraft(saved),
  });
  return (
    <Panel title="Profile">
      <PanelBody>
        <TextInput
          aria-label="Display name"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
      </PanelBody>
    </Panel>
  );
}

function ClaimPage() {
  const page = useSavePage();
  useUnsavedGuard(page?.dirty ?? false, undefined, page?.save);
  return null;
}

function Shell() {
  const [address, setAddress] = useState("account");
  return (
    <LocaleProvider initial="en">
      <Button onClick={() => setAddress("contacts")}>Go to Contacts</Button>
      <UnsavedGuard address={address} onKeep={setAddress}>
        {(shown) =>
          shown === "account" ? (
            <SavePageProvider>
              <ClaimPage />
              <NameCard />
              <PageSaveBar />
            </SavePageProvider>
          ) : (
            <p>Contacts</p>
          )
        }
      </UnsavedGuard>
    </LocaleProvider>
  );
}

export const LeaveDialog: Story = {
  render: () => <Shell />,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Go to Contacts" }),
    );
    const dialog = within(canvasElement.ownerDocument.body).getByRole("dialog");
    await expect(
      within(dialog).getByRole("button", { name: "Stay" }),
    ).toBeVisible();
  },
};
