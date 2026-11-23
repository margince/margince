// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { act, useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
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
      canvas.getByRole("textbox", { name: "Company name" }),
      "Havbris AS",
    );
    await userEvent.type(
      canvas.getByRole("textbox", { name: "Products and services" }),
      "Coastal ferry maintenance contracts.",
    );
    await userEvent.type(
      canvas.getByRole("textbox", { name: "Ideal customer" }),
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
    await user.type(
      drawer.getByRole("textbox", { name: "Company name" }),
      " Group",
    );
    // Awaited: Escape reaches the drawer through a window listener, and the
    // question it opens may suspend on the catalog inside the press's act.
    await act(() => user.keyboard("{Escape}"));
    const question = await screen.findByRole("dialog", {
      name: "Discard unsaved changes?",
    });
    await waitFor(() => expect(question).toBeVisible());
  },
};

export const EditDrawerDark: Story = {
  ...EditDrawer,
  globals: { theme: "dark" },
};
