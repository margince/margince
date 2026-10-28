// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  CircleCheck,
  CircleX,
  Info,
  Lightbulb,
  TriangleAlert,
} from "lucide-react";
import type { CSSProperties } from "react";
import {
  CheckCircleFilled,
  ErrorFilled,
  type Glyph,
  InfoFilled,
  Lightbulb2Filled,
  WarningFilled,
} from "./materialsymbols";
import { specimenColumn, specimenNote, specimenToken } from "./tokenspecimen";

const meta = {
  title: "Components/Images and icons/Material symbols",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const SYMBOLS: ReadonlyArray<
  Readonly<{ name: string; Mark: Glyph; line: Glyph; ink: string }>
> = [
  { name: "InfoFilled", Mark: InfoFilled, line: Info, ink: "--infoText" },
  {
    name: "WarningFilled",
    Mark: WarningFilled,
    line: TriangleAlert,
    ink: "--warningText",
  },
  {
    name: "ErrorFilled",
    Mark: ErrorFilled,
    line: CircleX,
    ink: "--dangerText",
  },
  {
    name: "CheckCircleFilled",
    Mark: CheckCircleFilled,
    line: CircleCheck,
    ink: "--successText",
  },
  {
    name: "Lightbulb2Filled",
    Mark: Lightbulb2Filled,
    line: Lightbulb,
    ink: "--discoveryText",
  },
];

const row: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--space-4)",
};

/** Each mark in its tone's ink, at the sizes a notice and a heading draw it. */
export const Marks: Story = {
  render: () => (
    <div className={specimenColumn}>
      {SYMBOLS.map(({ name, Mark, ink }) => (
        <div key={name} style={{ ...row, color: `var(${ink})` }}>
          <Mark size={16} />
          <Mark size={20} />
          <Mark size={24} />
          <code className={specimenToken}>{name}</code>
          <span className={specimenNote}>{ink}</span>
        </div>
      ))}
    </div>
  ),
};

/** The filled mark beside the lucide line glyph it can stand in for, both at
 * 16px, so the two read as one size on one line. */
export const BesideLucide: Story = {
  render: () => (
    <div className={specimenColumn}>
      {SYMBOLS.map(({ name, Mark, line: Line }) => (
        <div key={name} style={row}>
          <Mark size={16} />
          <Line size={16} />
          <code className={specimenToken}>{name}</code>
        </div>
      ))}
    </div>
  ),
};
