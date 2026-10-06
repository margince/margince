import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, within } from "storybook/test";
import { Field, Textarea, TextInput } from "./atoms";
import { ChoiceList } from "./choicelist";
import { ConfirmModal } from "./confirmmodal";

const meta: Meta = {
  title: "Components/Overlays and layering/Confirm modal",
  component: ConfirmModal,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function ConfirmDemo({
  tier,
  error,
}: Readonly<{ tier?: "confirm"; error?: string | null }>) {
  const [open, setOpen] = useState(true);
  return (
    <ConfirmModal
      open={open}
      onClose={() => setOpen(false)}
      title="Send this offer?"
      tier={tier}
      confirmLabel="Send"
      onConfirm={() => setOpen(false)}
      error={error}
    >
      <p>The buyer will receive the offer by email.</p>
    </ConfirmModal>
  );
}

const opensDialog = async () => {
  await screen.findByRole("dialog");
};

export const Default: Story = {
  render: () => <ConfirmDemo />,
  play: opensDialog,
};

export const ConfirmTier: Story = {
  render: () => <ConfirmDemo tier="confirm" />,
  play: opensDialog,
};

export const WithError: Story = {
  render: () => <ConfirmDemo error="A currency conversion rate is missing." />,
  play: opensDialog,
};

// Every row one stack step apart, the refusal included.
function ConfirmFormDemo({
  error,
  intent,
}: Readonly<{ error?: string; intent?: "drawer" }>) {
  const [open, setOpen] = useState(true);
  const [reason, setReason] = useState("price");
  return (
    <ConfirmModal
      open={open}
      onClose={() => setOpen(false)}
      title="Mark this deal lost?"
      confirmLabel="Mark lost"
      confirmVariant="danger"
      onConfirm={() => setOpen(false)}
      error={error}
      intent={intent}
    >
      <p>The deal leaves the pipeline and its open tasks close.</p>
      <ChoiceList
        legend="Why was it lost?"
        value={reason}
        choices={[
          { value: "price", label: "Price" },
          { value: "timing", label: "Timing" },
          { value: "competitor", label: "Went with a competitor" },
        ]}
        onChange={setReason}
      />
      <Field label="Competitor">
        {(control) => <TextInput {...control} defaultValue="Northwind" />}
      </Field>
      <Field label="Note" hint="Read by whoever picks the account up next.">
        {(control) => <Textarea {...control} rows={3} />}
      </Field>
    </ConfirmModal>
  );
}

const opensWithFields = async () => {
  const dialog = within(await screen.findByRole("dialog"));
  await expect(await dialog.findByLabelText("Competitor")).toBeInTheDocument();
  await expect(await dialog.findByLabelText("Note")).toBeInTheDocument();
};

export const WithFields: Story = {
  render: () => <ConfirmFormDemo />,
  play: opensWithFields,
};

export const WithFieldsAndError: Story = {
  render: () => (
    <ConfirmFormDemo error="The deal changed while you were here." />
  ),
  play: opensWithFields,
};

export const WithFieldsInDrawer: Story = {
  render: () => <ConfirmFormDemo intent="drawer" />,
  play: opensWithFields,
};
