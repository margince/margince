// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProblemError } from "../lib/problem";
import { StoryProviders } from "../testing/story-utils";
import { LeadHeldNotice } from "./leadheldnotice";

// A contact form's answer when a live lead already holds the address or the
// LinkedIn profile: convert the lead, open it, or ask its owner.
const meta: Meta = { title: "Patterns/Lead held notice" };
export default meta;

type Story = StoryObj;

function refusal(details: Record<string, unknown>) {
  return new ProblemError({ status: 409, code: "lead_holds_email", details });
}

function shown(details: Record<string, unknown>) {
  return (
    <StoryProviders>
      <LeadHeldNotice error={refusal(details)} onConvert={() => {}} />
    </StoryProviders>
  );
}

export const Convertible: Story = {
  render: () =>
    shown({ existing_id: "lead-1", lead_ids: ["lead-1"], convertible: true }),
};

export const MayNotConvert: Story = {
  render: () =>
    shown({ existing_id: "lead-1", lead_ids: ["lead-1"], convertible: false }),
};

export const HiddenLead: Story = { render: () => shown({}) };
