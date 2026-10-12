// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Card } from "./atoms";
import { NumberSetting } from "./numbersetting";
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
              onCommit={setSeconds}
            />
          )}
        />
      </SettingList>
    </Card>
  );
}

export const Default: Story = { render: () => <Limits /> };
