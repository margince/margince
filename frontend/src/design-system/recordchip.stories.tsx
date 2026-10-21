// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  RecordChip,
  type RecordChipKind,
  type RecordChipSize,
} from "./recordchip";

const meta: Meta<typeof RecordChip> = {
  title: "Components/Labels/Record chip",
  component: RecordChip,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RecordChip>;

function Sentence({
  initial,
  size = "default",
}: Readonly<{ initial: string | null; size?: RecordChipSize }>) {
  const [pressed, setPressed] = useState<string | null>(initial);
  const chip = (id: string, kind: RecordChipKind, label: string) => (
    <RecordChip
      kind={kind}
      label={label}
      identity={id}
      pressed={pressed === id}
      onPress={() => setPressed(pressed === id ? null : id)}
      size={size}
    />
  );
  const font = size === "large" ? "var(--fontLead)" : "var(--fontBodyLarge)";
  return (
    <p style={{ font }}>
      Start with {chip("c1", "contact", "Anna Weber")}. You see her at the{" "}
      {chip("m1", "meeting", "Weber GmbH review")} at 09:30, then{" "}
      {chip("k1", "company", "Northstar")}.
    </p>
  );
}

/** The three kinds in the line they are made for. */
export const InASentence: Story = { render: () => <Sentence initial={null} /> };

/** A pressed chip wears the strongest neutral edge: the line it opened under
 * the sentence is the one it was read from. */
export const Pressed: Story = { render: () => <Sentence initial="c1" /> };

/** Large, in the `--fontLead` sentence under the morning greeting. */
export const Large: Story = {
  render: () => <Sentence initial={null} size="large" />,
};
