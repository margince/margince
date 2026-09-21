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

// The press both open-list frames make, NAMED on each rather than inherited by
// one from the other.
//
// A story that picks up its `play` through a spread is indexed WITHOUT the
// `play-fn` tag — the indexer reads the object literal in front of it, not what
// the spread resolves to — and the capture gate keys its settle on that tag. So
// the dark frame was screenshotted 250ms after paint rather than 1.5s, which is
// before the list this story is named for has opened. The `render` can still be
// spread; nothing is keyed on it.
const openTheList: Story["play"] = async ({ canvasElement }) => {
  await userEvent.click(
    within(canvasElement).getByRole("combobox", { name: "Import window" }),
  );
  await within(canvasElement.ownerDocument.body).findByRole("option", {
    name: "10 years",
  });
};

export const Options: Story = { ...TenYears, play: openTheList };
export const OptionsDark: Story = {
  ...TenYears,
  play: openTheList,
  globals: { theme: "dark" },
};
