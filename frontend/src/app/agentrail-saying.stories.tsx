// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { StoryProviders } from "../testing/story-utils";
import { RailSaying } from "./agentrail-saying";
import { plain } from "./ai-activity-speak";
import "./agentrail.css";

const meta: Meta = {
  title: "Shell/Agent rail saying",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const FIRST = "Reading your mail.";
const SECOND = "Matching 3 contacts to a company.";

function Switching() {
  const [at, setAt] = useState(0);
  return (
    <StoryProviders>
      <p style={{ maxWidth: 220 }}>
        <RailSaying line={plain(at % 2 === 0 ? FIRST : SECOND)} />
      </p>
      <Button onClick={() => setAt(at + 1)}>Next reading</Button>
    </StoryProviders>
  );
}

/** A new reading crossfades over the last one in the same cell. */
export const NewReading: Story = {
  render: () => <Switching />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Next reading" }));
    await expect(await canvas.findByText(SECOND)).toBeInTheDocument();
    await expect(
      canvasElement.querySelectorAll(".argone").length,
    ).toBeLessThanOrEqual(1);
  },
};
