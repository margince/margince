// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { deal } from "../../testing/brief.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import {
  ExistingQualificationDeal,
  type QualificationDealChoice,
  qualificationDealState,
} from "./leads.qualifydeal";

const candidates = [
  deal("deal-one", "Annual renewal"),
  deal("deal-two", "New rollout"),
];
type Props = React.ComponentProps<typeof ExistingQualificationDeal>;
function Choice(props: Props) {
  const [choice, choose] = useState<QualificationDealChoice>(null);
  const state = qualificationDealState(
    "new",
    choice,
    props.preview,
    props.more,
  );
  return (
    <ExistingQualificationDeal
      {...props}
      selected={state.selectedDeal}
      choose={choose}
    />
  );
}
const meta: Meta<typeof ExistingQualificationDeal> = {
  title: "Records/Leads/Existing qualification deal",
  component: ExistingQualificationDeal,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  render: (args) => <Choice {...args} />,
  args: {
    preview: {
      outcome: "create",
      contact_withheld: false,
      company_id: "company-one",
      existing_deals: [candidates[0]].filter((entry) => entry !== undefined),
    },
    invalid: false,
    newDeal: false,
    more: false,
    fetching: false,
    loadMore: () => undefined,
    choose: () => undefined,
  },
};
export default meta;
type Story = StoryObj<typeof ExistingQualificationDeal>;
export const OneCandidate: Story = {};
export const OneCandidateDark: Story = { globals: { theme: "dark" } };
export const SeveralCandidates: Story = {
  args: {
    preview: {
      outcome: "create",
      contact_withheld: false,
      company_id: "company-one",
      existing_deals: candidates,
    },
  },
};
export const SeveralCandidatesDark: Story = {
  ...SeveralCandidates,
  globals: { theme: "dark" },
};
export const ChoiceUnavailable: Story = {
  args: {
    preview: { outcome: "create", contact_withheld: false },
    invalid: true,
  },
};
export const ChoiceUnavailableDark: Story = {
  ...ChoiceUnavailable,
  globals: { theme: "dark" },
};
export const FetchingAnotherPage: Story = {
  args: { more: true, fetching: true },
};
