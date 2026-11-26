// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders, storyQueryClient } from "../../testing/story-utils";
import {
  type RecipientCountry,
  useRecipientCountries,
} from "./composecountries";
import { RecipientCountryControl } from "./composecountry";

// A recipient's country on their chip. The record's answer is seeded in the
// cache, so the open panel shows it without a server.

const meta: Meta = {
  title: "Patterns/Compose mail/Recipient country",
};
export default meta;

type Story = StoryObj;

const ADDRESS = "anna@acme.test";

function Demo({ stated }: Readonly<{ stated?: RecipientCountry[] }>) {
  const countries = useRecipientCountries(stated);
  return <RecipientCountryControl address={ADDRESS} countries={countries} />;
}

function seeded() {
  const client = storyQueryClient();
  client.setQueryData(["compose-recipient-country", ADDRESS], {
    address: ADDRESS,
    country: "de",
    source: "employer_address",
  });
  return client;
}

export const FromTheRecord: Story = {
  render: () => (
    <StoryProviders client={seeded()}>
      <Demo />
    </StoryProviders>
  ),
};

export const SetForThisMessage: Story = {
  render: () => (
    <StoryProviders client={seeded()}>
      <Demo stated={[{ address: ADDRESS, country: "at" }]} />
    </StoryProviders>
  ),
};

export const ClearedDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders client={seeded()}>
      <Demo stated={[{ address: ADDRESS, country: null }]} />
    </StoryProviders>
  ),
};
