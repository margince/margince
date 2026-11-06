// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { installFetchStub, StoryProviders } from "../testing/story-utils";
import { PassportAreasDrawer } from "./passport-areas";
import type { PassportAreas, WorkArea } from "./workareas";

const ADMIN: WorkArea[] = [
  "business",
  "compliance",
  "capture",
  "workspace_settings",
  "admin_settings",
  "own_account",
];

function drawer(areas: PassportAreas) {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <PassportAreasDrawer
          subject={{ id: "pp-1", name: "Claude", areas }}
          reachable={ADMIN}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PassportAreasDrawer> = {
  title: "Settings/You/AI access/Change areas",
  component: PassportAreasDrawer,
};
export default meta;
type Story = StoryObj<typeof PassportAreasDrawer>;

// Opened on all areas: nothing to save yet, and no area is off.
export const FromAllAreas: Story = {
  render: drawer({ mode: "all", areas: [] }),
};

// Opened on chosen areas: the refusal sample names the first area that is off.
export const FromChosenAreas: Story = {
  render: drawer({ mode: "custom", areas: ["business", "own_account"] }),
};

export const FromChosenAreasDark: Story = {
  globals: { theme: "dark" },
  render: drawer({ mode: "custom", areas: ["business", "own_account"] }),
};
