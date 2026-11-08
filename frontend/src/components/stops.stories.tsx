// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ContactStops, LeadStops } from "./stops";

const meta: Meta = { title: "Records/Privacy/Contact and lead stops" };
export default meta;
type Story = StoryObj;
function stops(kind: "contact" | "lead", mayLift: boolean) {
  installFetchStub({
    [`GET /${kind}s/p-1/consent/stops`]: () =>
      jsonResponse({
        stops: [
          {
            id: "stop-1",
            kind: "marketing_objection",
            authority: "subject",
            reason: "No newsletters, please",
            recorded_at: "2026-10-02T09:00:00Z",
            recorded_by_colleague: true,
            recorded_by_name: "Rita Rep",
            may_lift: mayLift,
            correction: mayLift,
            ...(!mayLift ? { lift_blocked_by: "subject_decision" } : {}),
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      {kind === "contact" ? (
        <ContactStops contactId="p-1" />
      ) : (
        <LeadStops leadId="p-1" />
      )}
    </StoryProviders>
  );
}
export const Correctable: Story = {
  render: () => stops("contact", true),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).findByText("“No newsletters, please”"),
    ).resolves.toBeVisible();
  },
};
export const SubjectDecision: Story = {
  ...Correctable,
  render: () => stops("contact", false),
};
export const Lead: Story = {
  ...Correctable,
  render: () => stops("lead", true),
};
export const SubjectDecisionDark: Story = {
  ...SubjectDecision,
  globals: { theme: "dark" },
};
