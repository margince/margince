// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en as englishCopy } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import {
  CompanyProfileDialog,
  ManualCompanySetup,
} from "./company-context.profile";

const meta: Meta = {
  title: "Onboarding/Company profile forms",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type CompanyInput = components["schemas"]["CompanyProfileInput"];

/** The first-run form, empty: Create stays disabled until the minimum is in. */
export const ManualSetupEmpty: Story = {
  render: () => (
    <StoryProviders>
      <ManualCompanySetup />
    </StoryProviders>
  ),
};

/** The minimum filled in, one press from the save that opens Home. */
export const ManualSetupReady: Story = {
  ...ManualSetupEmpty,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      canvas.getByRole("textbox", {
        name: englishCopy["ob.field.display_name"],
      }),
      "Havbris AS",
    );
    await userEvent.type(
      canvas.getByRole("textbox", {
        name: englishCopy["ob.field.offer_summary"],
      }),
      "Coastal ferry maintenance contracts.",
    );
    await userEvent.type(
      canvas.getByRole("textbox", { name: englishCopy["ob.field.icp"] }),
      "Municipal ferry operators.",
    );
  },
};

export const ManualSetupDark: Story = {
  ...ManualSetupEmpty,
  globals: { theme: "dark" },
};

const SAVED_PROFILE: CompanyInput = {
  display_name: "Havbris AS",
  website: "havbris.example",
  offer_summary: "Coastal ferry maintenance contracts.",
  icp: "Municipal ferry operators.",
};

function EditProfile() {
  const [form, setForm] = useState<CompanyInput>(SAVED_PROFILE);
  return (
    <StoryProviders>
      <CompanyProfileDialog
        form={form}
        focus="offer_summary"
        pending={false}
        error={null}
        refusedAt={undefined}
        dirty={JSON.stringify(form) !== JSON.stringify(SAVED_PROFILE)}
        onChange={setForm}
        onClose={() => setForm(SAVED_PROFILE)}
        onSubmit={() => undefined}
      />
    </StoryProviders>
  );
}

/** The profile's one edit drawer, opened from the Products and services row. */
export const EditDrawer: Story = { render: () => <EditProfile /> };

// A changed draft and Escape: the drawer asks before it throws the draft away.
// Cancel discards at once.
export const EscapeAsksBeforeDiscarding: Story = {
  render: () => <EditProfile />,
  play: async () => {
    const user = userEvent.setup();
    const drawer = within(await screen.findByRole("dialog"));
    const name = drawer.getByRole("textbox", {
      name: englishCopy["ob.field.display_name"],
    });
    await user.type(name, " Group");
    // Dispatched by hand: Storybook wraps its key presses in act, and the
    // question opening under one suspends on the copy catalog and warns.
    name.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    const question = await screen.findByRole("dialog", {
      name: englishCopy["unsaved.title"],
    });
    await waitFor(() => expect(question).toBeVisible());
  },
};

export const EditDrawerDark: Story = {
  ...EditDrawer,
  globals: { theme: "dark" },
};
