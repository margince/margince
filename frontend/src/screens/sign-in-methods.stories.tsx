// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { SignInMethodsCard } from "./sign-in-methods";

// Password is present in every state and flippable in none: there is no value
// of the setting that removes it, so the row shows a control that exists and
// refuses rather than one that is missing.
const meta: Meta<typeof SignInMethodsCard> = {
  title: "Settings/Admin settings/General/Sign-in methods",
  component: SignInMethodsCard,
};
export default meta;

type Story = StoryObj<typeof SignInMethodsCard>;

/** A provider the deployment configured and the admin offers. */
export const ProviderEnabled: Story = {
  parameters: {
    signInProviders: [{ key: "google", label: "Google", enabled: true }],
  },
};

/** The same provider, switched off. It stays listed — the credentials are still
 * there, and the admin can put it back. */
export const ProviderDisabled: Story = {
  parameters: {
    signInProviders: [{ key: "google", label: "Google", enabled: false }],
  },
};

/** No external provider configured, so password is the whole answer. */
export const PasswordOnly: Story = {
  parameters: { signInProviders: [] },
};
