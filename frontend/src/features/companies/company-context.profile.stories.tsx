// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
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
    await userEvent.type(canvas.getByLabelText("Company name"), "Havbris AS");
    await userEvent.type(
      canvas.getByLabelText("Products and services"),
      "Coastal ferry maintenance contracts.",
    );
    await userEvent.type(
      canvas.getByLabelText("Ideal customer"),
      "Municipal ferry operators.",
    );
  },
};

export const ManualSetupDark: Story = {
  ...ManualSetupEmpty,
  globals: { theme: "dark" },
};

function EditProfile() {
  const [form, setForm] = useState<CompanyInput>({
    display_name: "Havbris AS",
    website: "havbris.example",
    offer_summary: "Coastal ferry maintenance contracts.",
    icp: "Municipal ferry operators.",
  });
  return (
    <StoryProviders>
      <CompanyProfileDialog
        form={form}
        focus="offer_summary"
        pending={false}
        error={null}
        onChange={setForm}
        onClose={() => undefined}
        onSubmit={() => undefined}
      />
    </StoryProviders>
  );
}

/** The profile's one edit drawer, opened from the Products and services row. */
export const EditDrawer: Story = { render: () => <EditProfile /> };

export const EditDrawerDark: Story = {
  ...EditDrawer,
  globals: { theme: "dark" },
};
