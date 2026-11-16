// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskAssigneeField } from "./assigneepicker";

const members = Array.from({ length: 14 }, (_, index) => ({
  id: `u-${index}`,
  display_name: `Member ${index}`,
  email: `member${index}@example.test`,
  is_agent: false,
}));
function Assignee() {
  const [value, setValue] = useState("u-0");
  return <TaskAssigneeField active value={value} onChange={setValue} />;
}
const meta: Meta = {
  title: "Patterns/Task assignee",
  component: TaskAssigneeField,
};
export default meta;
type Story = StoryObj;
export const SearchByEmail: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["create"] }),
      "GET /users": () =>
        jsonResponse({
          data: members,
          page: { has_more: false, next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <Assignee />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const trigger = await within(canvasElement).findByRole("combobox");
    await waitFor(() => expect(trigger).toHaveTextContent("Member 0"));
    await user.click(trigger);
    const body = within(document.body);
    await user.type(
      body.getByRole("combobox", { name: "Filter the list" }),
      "member13@",
    );
    await user.click(body.getByRole("option", { name: "Member 13" }));
    await expect(trigger).toHaveTextContent("Member 13");
  },
};
export const SearchByEmailDark: Story = {
  ...SearchByEmail,
  globals: { theme: "dark" },
};
export const SearchByEmailPhone: Story = {
  ...SearchByEmail,
  tags: ["uat-phone"],
};
