import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import { MoneyInput } from "./moneyinput";

const meta: Meta = {
  title: "Design System/MoneyInput",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function MoneyDemo() {
  const { locale } = useLocale();
  const [minor, setMinor] = useState(150_000);
  return (
    <div style={{ maxWidth: 200 }}>
      <MoneyInput
        currency="EUR"
        valueMinor={minor}
        onChangeMinor={setMinor}
        aria-label="Unit price"
      />
      <p style={{ marginTop: "var(--space-2)" }}>
        minor units: {formatNumber(minor, locale)}
      </p>
    </div>
  );
}

export const Default: Story = {
  render: () => <MoneyDemo />,
};
