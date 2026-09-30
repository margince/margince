// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ManualCompanySetup } from "./company-context";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The floor below the `onboarding` rollout stage never asks for the rollout's
// capabilities, so these stories route no capability read.
const meta: Meta = {
  title: "Onboarding/Manual company setup",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function ManualSetup() {
  return (
    <StoryProviders>
      <ManualCompanySetup />
    </StoryProviders>
  );
}

export const ManualSetupDefault: Story = {
  render: () => <ManualSetup />,
};

// The reviewer fills the semantic minimum and submits, but the workspace
// PUT fails server-side (a duplicate domain, a validation the client can't
// see), the one branch that shows the form's own error paragraph rather
// than a disabled button.
export const ManualSetupSaveFailed: Story = {
  render: () => {
    installFetchStub({
      "PUT /company": () =>
        jsonResponse(
          { title: "A workspace already exists for this domain." },
          409,
        ),
    });
    return <ManualSetup />;
  },
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
    await userEvent.click(
      canvas.getByRole("button", { name: /Create company context/ }),
    );
    await canvas.findByText(/already exists for this domain/);
  },
};
