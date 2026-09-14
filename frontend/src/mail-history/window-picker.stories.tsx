import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ImportWindowPicker } from "./window-picker";

type ImportWindow = components["schemas"]["StartBackfillRequest"]["window"];
const meta: Meta<typeof ImportWindowPicker> = {
  title: "Settings/Connections/Import window",
  component: ImportWindowPicker,
};
export default meta;
type Story = StoryObj<typeof ImportWindowPicker>;
function Picker() {
  const [value, setValue] = useState<ImportWindow>("120m");
  return (
    <LocaleProvider initial="en">
      <ImportWindowPicker
        value={value}
        onChange={setValue}
        preview={{
          window: "120m",
          after_date: "2016-09-14",
          computed_at: "2026-09-14T12:00:00Z",
          estimated_messages: 20000,
          estimate_is_floor: true,
        }}
      />
    </LocaleProvider>
  );
}
export const TenYears: Story = { render: () => <Picker /> };
export const TenYearsDark: Story = { ...TenYears, globals: { theme: "dark" } };

export const Options: Story = {
  ...TenYears,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("combobox", { name: "Import window" }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("option", {
      name: "10 years",
    });
  },
};
export const OptionsDark: Story = { ...Options, globals: { theme: "dark" } };
