// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Panel, PanelGroupHead } from "../design-system/panel";
import { GrantMatrix, type GrantMatrixRow } from "./grantmatrix";
import { StoryProviders } from "./story-utils";

const NONE = { read: false, create: false, update: false, delete: false };

function row(
  key: string,
  name: string,
  grant: Partial<typeof NONE> = {},
  badge?: string,
): GrantMatrixRow {
  return {
    key,
    name,
    badge,
    grant: { ...NONE, ...grant },
    cellLabel: (action) => `Allow ${name} to ${action} Endpoint`,
    pending: false,
    onChange: () => {},
  };
}

const ROWS = [
  row("admin", "Admin", { read: true, create: true, update: true }),
  row("rep", "User", { read: true }),
  row("ops", "Ops / Integrations"),
  row(
    "partner",
    "Regional partner channel coordinator for DACH and Benelux",
    {},
    "Custom role",
  ),
];

function story(canManage: boolean) {
  return () => (
    <StoryProviders>
      <Panel title="openchannel">
        <PanelGroupHead title="Endpoint" level="h3" />
        <GrantMatrix
          rowHeader="Role"
          rows={ROWS}
          canManage={canManage}
          readOnlyReason="Your role can read this page. Changing a grant requires a full seat."
          scrollLabel="Endpoint"
          bleed
        />
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof GrantMatrix> = {
  title: "Patterns/Grant matrix",
  component: GrantMatrix,
};
export default meta;
type Story = StoryObj<typeof GrantMatrix>;

export const Editable: Story = { render: story(true) };

// Every switch refused, its reason read out and kept out of sight.
export const ReadOnly: Story = { render: story(false) };

// The role column stays while the verbs scroll under it, and the page does
// not pan.
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(false),
  play: async ({ canvasElement }) => {
    const matrix = await within(canvasElement).findByRole("table");
    const card = matrix.closest<HTMLElement>(".panel");
    const scroller = matrix.closest<HTMLElement>(".table-scroll");
    if (!card || !scroller) throw new Error("the matrix left its pane");
    await expect(card.scrollWidth).toBeLessThanOrEqual(card.clientWidth);
    for (const hidden of matrix.querySelectorAll<HTMLElement>(
      ".switchreason",
    )) {
      await expect(scroller.contains(hidden.offsetParent)).toBe(true);
    }
  },
};
