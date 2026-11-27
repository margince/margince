// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { informationReport } from "./systemhealth.fixtures";
import { HealthChecks, healthGroups } from "./systemhealthchecks";

const meta = {
  title: "Settings/System/System health/Check groups",
  component: HealthChecks,
  args: {
    groups: healthGroups(informationReport.sources, informationReport.findings),
  },
  render: (args) => {
    installFetchStub({ "GET /me": meRoute({ ai_diagnostics: ["read"] }) });
    return (
      <StoryProviders>
        <HealthChecks {...args} />
      </StoryProviders>
    );
  },
} satisfies Meta<typeof HealthChecks>;
export default meta;
type Story = StoryObj<typeof meta>;
export const WithInformation: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("list", { name: englishCopy["health.group.setup"] }),
    ).toBeVisible();
    const information = canvas.getByRole("list", {
      name: englishCopy["health.group.information"],
    });
    await expect(
      within(information).getByText(englishCopy["health.source.unplaced"]),
    ).toBeVisible();
    await expect(
      within(canvas.getByRole("list", { name: "Passing" })).queryByText(
        englishCopy["health.source.unplaced"],
      ),
    ).toBeNull();
  },
};
