// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { within } from "storybook/test";
import { taskRow } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { expectTextFits } from "../../testing/unbrokentext";
import { TaskCard } from "./brief.taskcard";
import "./brief.feed.css";

const meta: Meta<typeof TaskCard> = {
  title: "Shell/Home/Task card",
  component: TaskCard,
};
export default meta;
type Story = StoryObj<typeof TaskCard>;

function card(item: ComponentProps<typeof TaskCard>["item"]) {
  return () => {
    installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
    return (
      <StoryProviders>
        <TaskCard item={item} onOpen={() => undefined} />
      </StoryProviders>
    );
  };
}

const LONG_CONTACT = "Maximilian Konstantinopoulou-Wertheimer";

/** A contact's long name breaks inside its chip and stays on the card. */
export const LongContactNamePhone: Story = {
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
  render: card({
    ...taskRow("call-max", "Call back about the renewal"),
    contact: {
      id: "c-max",
      label: LONG_CONTACT,
      employer: { company_id: "co-weber", company_name: "Weber GmbH" },
    },
  }),
  play: async ({ canvasElement }) => {
    const article = canvasElement.querySelector("article");
    const chip = within(canvasElement)
      .getAllByText(LONG_CONTACT)
      .map((name) => name.closest(".chip"))
      .find((found) => found !== null);
    if (!article || !chip)
      throw new Error("the contact has no chip on the card");
    await expectTextFits(chip, article.getBoundingClientRect());
  },
};

export const LongContactNamePhoneDark: Story = {
  ...LongContactNamePhone,
  globals: { ...LongContactNamePhone.globals, theme: "dark" },
};
