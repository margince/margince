// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { ListSurface } from "../../design-system/listsurface";
import { LocaleProvider, useLocale, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { dealRangeChips } from "./deals.rangefilters";

const meta: Meta = {
  title: "Records/Deals/Commercial filters",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The filter bar with the close-date, value, currency and country dials, the
// way the deal list hands them to its surface.
function Bar({
  initial = {},
  withheld = new Set(),
}: Readonly<{
  initial?: Record<string, string>;
  withheld?: ReadonlySet<string>;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const [chosen, setChosen] = useState(initial);
  return (
    <div style={{ blockSize: "34rem" }}>
      <ListSurface
        count="12 deals"
        chips={dealRangeChips({
          words: { t, locale, zone },
          filters: chosen,
          baseCurrency: "EUR",
          withheld,
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

/** The Filters step of the View panel, open and drawn. */
async function openFilters(canvasElement: HTMLElement): Promise<HTMLElement> {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name: /^View/ }),
  );
  const panel = await screen.findByRole("dialog");
  await waitFor(() => expect(panel).toBeVisible());
  await userEvent.click(
    within(panel).getByRole("button", { name: /^Filters,/ }),
  );
  return panel;
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
    const panel = within(await openFilters(canvasElement));
    await userEvent.click(
      await panel.findByRole("button", { name: "Close date" }),
    );
    await expect(
      await panel.findByRole("radio", { name: "This fiscal quarter" }),
    ).toBeVisible();
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

// A role that withholds the amount and the company: only the close date is
// offered, since the server refuses the other three dials.
export const AmountAndCompanyWithheld: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <Bar withheld={new Set(["value", "currency", "country"])} />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const panel = within(await openFilters(canvasElement));
    await expect(
      await panel.findByRole("button", { name: "Close date" }),
    ).toBeVisible();
    for (const name of ["Value", "Currency", "Country"]) {
      await expect(panel.queryByRole("button", { name })).toBeNull();
    }
  },
};
