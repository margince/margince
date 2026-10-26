// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import { ListSurface } from "../../design-system/listsurface";
import { LocaleProvider, useLocale, useT } from "../../i18n";
import { dealRangeChips } from "./deals.rangefilters";

const meta: Meta = {
  title: "Records/Deals/Commercial filters",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The filter bar with the close-date, value, currency and country dials, the
// way the deal list hands them to its surface.
function Bar({ initial = {} }: Readonly<{ initial?: Record<string, string> }>) {
  const t = useT();
  const { locale } = useLocale();
  const [chosen, setChosen] = useState(initial);
  return (
    <div style={{ blockSize: "34rem" }}>
      <ListSurface
        count="12 deals"
        chips={dealRangeChips({
          words: { t, locale, zone: "Europe/Berlin" },
          filters: chosen,
          baseCurrency: "EUR",
          leading: ["EUR", "USD", "VND"],
        })}
        chosen={chosen}
        onChipChange={(key, value) =>
          setChosen((prev) => ({ ...prev, [key]: value }))
        }
      >
        <p>rows</p>
      </ListSurface>
    </div>
  );
}

// The close-date step open: "this fiscal quarter" as one press, or two days
// typed or picked in the calendar.
export const CloseDateStep: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <Bar />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Filter" }),
    );
    await userEvent.click(canvas.getByRole("button", { name: "Close date" }));
    await expect(
      canvas.getByRole("radio", { name: "This fiscal quarter" }),
    ).toBeTruthy();
    await expect(screen.getByRole("button", { name: "Apply" })).toBeTruthy();
  },
};

// Every dial applied, as a saved view or an address restores them.
export const Applied: Story = {
  render: () => (
    <LocaleProvider initial="vi">
      <Bar
        initial={{
          close_day: "this_quarter",
          value: "2000000..",
          currency: "EUR",
          country: "VN",
        }}
      />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("group", {
        name: "Quốc gia: Việt Nam",
      }),
    ).toBeTruthy();
  },
};
