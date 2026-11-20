// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { StoryProviders } from "../testing/story-utils";
import { LawfulBasisCallout } from "./lawfulbasiswarning";

const meta: Meta<typeof LawfulBasisCallout> = {
  title: "Patterns/Lawful basis warning",
  component: LawfulBasisCallout,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof LawfulBasisCallout>;
type Preview = components["schemas"]["SendAuthorizationPreview"];

const covered: Preview = {
  allowed: true,
  recipients: [
    { address: "alice@demo.test", verdict: "allow", reason_code: "allowed" },
  ],
};

const uncovered: Preview = {
  allowed: true,
  recipients: [
    { address: "alice@demo.test", verdict: "allow", reason_code: "allowed" },
    {
      address: "bob@demo.test",
      verdict: "allow",
      reason_code: "allowed",
      lawful_basis_missing: true,
    },
  ],
};

// Nothing is drawn: every recipient has a basis on record.
export const EveryRecipientCovered: Story = { args: { preview: covered } };

export const RecipientWithoutBasis: Story = { args: { preview: uncovered } };
