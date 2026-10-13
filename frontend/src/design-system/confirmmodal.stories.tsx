import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
  cancelLabel,
}: Readonly<{
  tier?: "confirm";
  error?: string | null;
  cancelLabel?: string;
}>) {
  const [open, setOpen] = useState(true);
  return (
    <ConfirmModal
      open={open}
      onClose={() => setOpen(false)}
      title="Send this offer?"
      tier={tier}
      confirmLabel="Send"
      cancelLabel={cancelLabel}
      onConfirm={() => setOpen(false)}
      error={error}
    >
      <p>The buyer will receive the offer by email.</p>
    </ConfirmModal>
  );
}

export const Default: Story = {
  render: () => <ConfirmDemo />,
};

export const ConfirmTier: Story = {
  render: () => <ConfirmDemo tier="confirm" />,
};

export const WithError: Story = {
  render: () => <ConfirmDemo error="A currency conversion rate is missing." />,
};

/** The way back named for what it keeps, where Cancel would not say it. */
export const NamedCancel: Story = {
  render: () => <ConfirmDemo cancelLabel="Keep the draft" />,
};
