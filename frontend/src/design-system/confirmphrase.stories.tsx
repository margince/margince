// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef, useState } from "react";
import { ConfirmModal } from "./confirmmodal";
import { ConfirmPhraseBox, phraseMatches } from "./confirmphrase";

// A typed confirmation, empty, part typed and matched, then seated in the
// dialog that gates its confirm on the match.

const meta: Meta<typeof ConfirmPhraseBox> = {
  title: "Components/Forms and input/Confirm phrase box",
  component: ConfirmPhraseBox,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ConfirmPhraseBox>;

const COMPANY = "Brandt Automotive GmbH";
const IMPACT = [
  "Every contact, company, lead, deal and offer, and all captured mail and meetings.",
  "Every mailbox and calendar connection. Everyone connects again afterwards.",
  "Users, roles, teams, company settings and the audit log stay.",
];

function Live({ initial }: Readonly<{ initial: string }>) {
  const [value, setValue] = useState(initial);
  return (
    <ConfirmPhraseBox
      title={`This permanently deletes the data of everyone at ${COMPANY}`}
      impact={IMPACT}
      prompt="Type this company name:"
      phrase={COMPANY}
      value={value}
      onChange={setValue}
    />
  );
}

export const Empty: Story = { render: () => <Live initial="" /> };
export const PartlyTyped: Story = {
  render: () => <Live initial="Brandt Au" />,
};
export const Matched: Story = { render: () => <Live initial={COMPANY} /> };
export const MatchedDark: Story = { ...Matched, globals: { theme: "dark" } };

// No title: the dialog already says what happens, so there is no plate.
function FieldOnlyBox() {
  const [value, setValue] = useState("");
  return (
    <ConfirmPhraseBox
      prompt="Type this word to confirm:"
      phrase="ERASE"
      value={value}
      onChange={setValue}
    />
  );
}

export const FieldOnly: Story = { render: () => <FieldOnlyBox /> };

function InDialog() {
  const [typed, setTyped] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  return (
    <ConfirmModal
      open
      onClose={() => setTyped("")}
      title={`Reset all data for ${COMPANY}?`}
      confirmLabel="Delete all data"
      confirmVariant="danger"
      confirmDisabled={!phraseMatches(typed, COMPANY)}
      initialFocusTo={() => inputRef.current}
      onConfirm={() => setTyped("")}
    >
      <ConfirmPhraseBox
        title={`This permanently deletes the data of everyone at ${COMPANY}`}
        impact={IMPACT}
        prompt="Type this company name:"
        phrase={COMPANY}
        value={typed}
        onChange={setTyped}
        inputRef={inputRef}
      />
    </ConfirmModal>
  );
}

export const InsideConfirmModal: Story = { render: () => <InDialog /> };
export const InsideConfirmModalDark: Story = {
  ...InsideConfirmModal,
  globals: { theme: "dark" },
};
