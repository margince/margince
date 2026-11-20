// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { DsrClosing } from "../../lib/privacy.logic";
import { StoryProviders } from "../../testing/story-utils";
import { DsrProofFields, DsrTransitions } from "./privacy.requestproof";

// What a close records beside the answer, and the verbs that close: as drafted,
// and with a requester address the close refused on its field.

const DRAFT: DsrClosing = {
  resolution: "Sent the export.",
  identityCheck: "confirm_link",
  requesterEmail: "",
};

function Closing({
  initial,
  emailError,
}: Readonly<{ initial: DsrClosing; emailError?: string }>) {
  const [closing, setClosing] = useState(initial);
  return (
    <StoryProviders>
      <div className="form-stack">
        <DsrProofFields
          closing={closing}
          emailError={emailError}
          onDraft={(change) => setClosing((was) => ({ ...was, ...change }))}
        />
        <DsrTransitions
          status="in_progress"
          ready={() => true}
          pending={false}
          onTransition={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof DsrProofFields> = {
  title: "Settings/Compliance/Privacy and retention/Closing a subject request",
  component: DsrProofFields,
};
export default meta;
type Story = StoryObj<typeof DsrProofFields>;

export const Drafted: Story = { render: () => <Closing initial={DRAFT} /> };

/** The address is not one, said on its field before anything is sent. */
export const EmailRefused: Story = {
  render: () => (
    <Closing
      initial={{ ...DRAFT, requesterEmail: "not-an-email" }}
      emailError="Enter an email address for Requester’s email address."
    />
  ),
};

export const EmailRefusedDark: Story = {
  ...EmailRefused,
  globals: { theme: "dark" },
};
