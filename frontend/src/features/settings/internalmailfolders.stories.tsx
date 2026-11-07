// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { InternalMailFolders } from "./internalmailfolders";

const meta = {
  title: "Settings/Data/Capture rules/Folder scope",
  component: InternalMailFolders,
} satisfies Meta<typeof InternalMailFolders>;
export default meta;
type Story = StoryObj<typeof meta>;
function Folders(args: Parameters<typeof InternalMailFolders>[0]) {
  const [values, setValues] = useState([...args.values]);
  return <InternalMailFolders {...args} values={values} onChange={setValues} />;
}
export const Available: Story = {
  args: {
    source: {
      connection_id: "connection-1",
      owner_user_id: "user-1",
      provider: "graph",
      generation: 1,
      account_label: "operations@demo.test",
      owner_name: "Ana Fischer",
    },
    values: [],
    disabled: false,
    onChange: () => {},
  },
  render: (args) => {
    installFetchStub({
      "GET /connectors/graph/containers": () =>
        jsonResponse({
          containers: [{ id: "operations", name: "Customer Operations" }],
          truncated: false,
        }),
    });
    return (
      <StoryProviders>
        <Folders {...args} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button"));
    await expect(canvas.findByRole("group")).resolves.toBeVisible();
  },
};
export const MissingFolder: Story = {
  ...Available,
  args: { ...Available.args, values: ["graph:removed"] },
};
export const AvailableDark: Story = {
  ...Available,
  globals: { theme: "dark" },
};
