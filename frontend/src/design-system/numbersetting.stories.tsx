// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, within } from "storybook/test";
import { formatInterval } from "../format/interval";
import { Card } from "./atoms";
import { Callout } from "./callout";
import { NumberSetting, NumberSettingRow } from "./numbersetting";
import { SettingList, SettingRow } from "./settingrow";

const meta: Meta<typeof NumberSetting> = {
  title: "Components/Forms and input/Number setting",
  component: NumberSetting,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof NumberSetting>;

/**
 * Two bounded numbers in one card. Type a value past either end and leave the
 * box: the value stays, with the refusal under it.
 */
function Limits() {
  const [pages, setPages] = useState(60);
  const [seconds, setSeconds] = useState(240);
  return (
    <Card title="Website reads">
      <SettingList>
        <SettingRow
          label="Pages per read"
          description="Most pages one read fetches, 1 to 200."
          control={(control) => (
            <NumberSetting
              control={control}
              value={pages}
              min={1}
              max={200}
              refusal="Enter a whole number of pages from 1 to 200."
              onCommit={setPages}
            />
          )}
        />
        <SettingRow
          label="Read time (seconds)"
          description="Longest one read runs, 30 to 600 seconds."
          control={(control) => (
            <NumberSetting
              control={control}
              value={seconds}
              min={30}
              max={600}
              refusal="Enter a whole number of seconds from 30 to 600."
              reading={(typed) => formatInterval(typed, "en")}
              onCommit={setSeconds}
            />
          )}
        />
      </SettingList>
    </Card>
  );
}

export const Default: Story = { render: () => <Limits /> };

/** A reader who may not change the number reads why under the box. */
export const NotYours: Story = {
  render: () => (
    <Card title="Website reads">
      <SettingList>
        <SettingRow
          label="Pages per read"
          description="Most pages one read fetches, 1 to 200."
          control={(control) => (
            <NumberSetting
              control={control}
              value={60}
              min={1}
              max={200}
              refusal="Enter a whole number of pages from 1 to 200."
              reason="Only an administrator or operations user can change this."
              onCommit={() => {}}
            />
          )}
        />
      </SettingList>
    </Card>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("textbox", { name: "Pages per read" }),
    ).toBeDisabled();
    await expect(
      canvas.getByText(/Only an administrator or operations user/),
    ).toBeVisible();
  },
};

export const NamesCallerElement: Story = {
  render: () => (
    <Card title="Website reads">
      <SettingList>
        <NumberSettingRow
          label="Pages per read"
          description="Most pages one read fetches, 1 to 200."
          value={60}
          min={1}
          max={200}
          refusal="Enter a whole number of pages from 1 to 200."
          describedBy="pages-refused"
          onCommit={() => {}}
        />
      </SettingList>
      <Callout
        id="pages-refused"
        kind="outcome"
        tone="danger"
        title="Couldn't save the setting"
      >
        You don't have permission to do that.
      </Callout>
    </Card>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("textbox", { name: "Pages per read" }),
    ).toHaveAccessibleDescription(
      expect.stringMatching(/^Most pages one read fetches.*Couldn't save/),
    );
  },
};
