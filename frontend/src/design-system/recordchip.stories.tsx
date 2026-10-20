// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import { RecordChip, type RecordChipKind } from "./recordchip";

const meta: Meta<typeof RecordChip> = {
  title: "Components/Labels/Record chip",
  component: RecordChip,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RecordChip>;

function Sentence({ initial }: Readonly<{ initial: string | null }>) {
  const [pressed, setPressed] = useState<string | null>(initial);
  const chip = (id: string, kind: RecordChipKind, label: string) => (
    <RecordChip
      kind={kind}
      label={label}
      identity={id}
      pressed={pressed === id}
      onPress={() => setPressed(pressed === id ? null : id)}
    />
  );
  return (
    <p style={{ font: "var(--fontBodyLarge)" }}>
      Start with {chip("c1", "contact", "Anna Weber")}. You see her at the{" "}
      {chip("m1", "meeting", "Weber GmbH review")} at 09:30, then{" "}
      {chip("k1", "company", "Northstar")}.
    </p>
  );
}

/** The three kinds in the line they are made for. */
export const InASentence: Story = { render: () => <Sentence initial={null} /> };

/** A pressed chip wears the AI edge: the quote it opened is what the brief was
 * read from. */
export const Pressed: Story = { render: () => <Sentence initial="c1" /> };
