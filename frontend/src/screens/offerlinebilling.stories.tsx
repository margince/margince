// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  EMPTY_LINE_BILLING,
  type LineBilling,
  OfferLineBillingFields,
} from "./offerlinebilling";
import { StoryProviders } from "./story-utils";
import "./offers.css";

// The billing controls on an offer's add-line form, laid out in the same row
// the form draws them in. Only a recurring line shows the cadence and the
// committed term, so the two stories are the two shapes the row can take.

function Harness({ initial }: Readonly<{ initial: LineBilling }>) {
  const [value, setValue] = useState(initial);
  return (
    <div className="offers-new-line">
      <OfferLineBillingFields value={value} onChange={setValue} />
    </div>
  );
}

const meta: Meta<typeof Harness> = {
  title: "Records/Offers/Line billing",
  component: Harness,
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

type Story = StoryObj<typeof Harness>;

export const Unclassified: Story = {
  args: { initial: EMPTY_LINE_BILLING },
};

export const Recurring: Story = {
  args: {
    initial: {
      billingModel: "recurring",
      billingIntervalMonths: "12",
      intervalCount: "3",
    },
  },
};
