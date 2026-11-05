// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import {
  expectAppliedFiltersFit,
  expectAppliedFiltersOneLine,
  LONG_FILTER_CHIPS,
  LONG_FILTER_VALUES,
  WIDE,
} from "../testing/appliedfilterchecks";
import { FilterRow } from "./listfilter";
import type { ListChip } from "./listsurface.dials";

// An applied filter as the toolbar draws it. A long value wraps at its spaces,
// and the "⋮" that removes the filter never leaves the row.

function Rows({ chips }: Readonly<{ chips: readonly ListChip[] }>) {
  return (
    <div className="lt-tools">
      {chips.map((chip) => (
        <FilterRow
          key={chip.key}
          chip={chip}
          value={LONG_FILTER_VALUES[chip.key] ?? chip.options[0]?.value ?? ""}
          openSegment={null}
          onToggleSegment={() => undefined}
          onPick={() => undefined}
          onDelete={() => undefined}
        />
      ))}
    </div>
  );
}

const meta = {
  title: "Components/Forms and input/Applied filter",
  component: Rows,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="de">
        <Story />
      </LocaleProvider>
    ),
  ],
} satisfies Meta<typeof Rows>;
export default meta;

type Story = StoryObj<typeof meta>;

export const ShortValue: Story = {
  args: {
    chips: [
      {
        key: "country",
        label: "Land",
        allLabel: "Jedes Land",
        options: [{ value: "DE", label: "Deutschland" }],
      },
    ],
  },
};

export const LongValuePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  args: { chips: LONG_FILTER_CHIPS },
  play: async ({ canvasElement }) => expectAppliedFiltersFit(canvasElement),
};

export const LongValueWide: Story = {
  parameters: WIDE,
  globals: { viewport: { value: "wide" } },
  tags: ["uat-wide"],
  args: { chips: LONG_FILTER_CHIPS },
  play: async ({ canvasElement }) => expectAppliedFiltersOneLine(canvasElement),
};
