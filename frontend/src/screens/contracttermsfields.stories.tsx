// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { screen } from "storybook/test";
import { Modal } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import type { ContractDraft } from "./contractform";
import { ContractTermsFields } from "./contracttermsfields";
import { StoryProviders } from "./story-utils";

// The component spaces its own rows, whichever dialog holds it.

const meta: Meta = {
  title: "Records/Company 360/Contract terms",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DRAFT: ContractDraft = {
  title: "Northwind Handel GmbH — Rahmenvertrag",
  contractNumber: "NW-2026-014",
  valueMinor: 15_000_000,
  arrMinor: 12_000_000,
  currency: "EUR",
  valueBasis: "annualized_12m",
  startsOn: "2025-10-23",
  endsOn: "2026-10-23",
  renewalOn: "",
  noticePeriodDays: "90",
  paymentTermDays: "30",
  signedOn: "2025-10-09",
  customValues: {},
};

function Terms() {
  const [draft, setDraft] = useState(DRAFT);
  return (
    <StoryProviders>
      <Modal open onClose={() => setDraft(DRAFT)} labelledBy="terms-title">
        <Heading size="large" id="terms-title" className="modal-title">
          {DRAFT.title}
        </Heading>
        <ContractTermsFields
          draft={draft}
          setDraft={setDraft}
          currency={draft.currency}
        />
      </Modal>
    </StoryProviders>
  );
}

export const Filled: Story = {
  render: () => <Terms />,
  play: async () => {
    await screen.findByRole("dialog");
  },
};

export const FilledDark: Story = {
  ...Filled,
  globals: { theme: "dark" },
};
