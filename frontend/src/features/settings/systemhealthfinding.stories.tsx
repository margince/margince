// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { overdueFinding } from "./systemhealth.fixtures";
import { FindingPanel } from "./systemhealthfinding";

const meta = {
  title: "Settings/Governance/System health/Finding",
  component: FindingPanel,
  args: { finding: overdueFinding },
  render: (args) => {
    installFetchStub({ "GET /me": meRoute({ job_health: ["read"] }) });
    return (
      <StoryProviders>
        <FindingPanel {...args} />
      </StoryProviders>
    );
  },
} satisfies Meta<typeof FindingPanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Overdue: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
