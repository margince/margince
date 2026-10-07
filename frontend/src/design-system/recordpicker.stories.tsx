import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Field } from "./atoms";
import { RecordPicker, type RecordPickerCandidate } from "./recordpicker";

// Stories are the render surface the change-scoped fe-uat capture gate drives
// (frontend/scripts/fe-uat.mjs).
// This is RecordPicker's first caller: a fixed in-memory candidate list
// stands in for a real search transport until the offer header and
// line-item pickers wire a live one.
const meta: Meta = {
  title: "Components/Forms and input/Record picker",
  component: RecordPicker,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const fixtureCandidates: RecordPickerCandidate[] = [
  { id: "company-1", name: "Brandt Automotive" },
  { id: "company-2", name: "Weber Logistics" },
  { id: "company-3", name: "Fischer & Wagner" },
];

function searchFixture(q: string): Promise<RecordPickerCandidate[]> {
  const needle = q.toLowerCase();
  return Promise.resolve(
    fixtureCandidates.filter((candidate) =>
      candidate.name.toLowerCase().includes(needle),
    ),
  );
}

function PickerDemo() {
  const [selected, setSelected] = useState<RecordPickerCandidate | null>(null);
  return (
    <div style={{ maxWidth: 320 }}>
      <RecordPicker
        label="Search companies…"
        searchTargets={searchFixture}
        onPick={setSelected}
        selected={selected}
      />
      {selected && <p style={{ marginTop: 8 }}>Picked: {selected.name}</p>}
    </div>
  );
}

export const Default: Story = {
  render: () => <PickerDemo />,
};

export const LabelAndHelp: Story = {
  render: () => (
    <Field label="Parent company" hint="Choose an existing company.">
      {(control) => (
        <RecordPicker
          {...control}
          searchTargets={searchFixture}
          onPick={() => undefined}
        />
      )}
    </Field>
  ),
};
