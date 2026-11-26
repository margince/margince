import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { formatAnalyticsMoney, formatNumber } from "../format/format";
import { LocaleProvider, useLocale } from "../i18n";
import { MoneyInput } from "./moneyinput";

const meta: Meta = {
  title: "Components/Forms and input/Money input",
  component: MoneyInput,
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

function DisplayDemo() {
  const { locale } = useLocale();
  const [minor, setMinor] = useState(1_500_000);
  return (
    <div style={{ maxWidth: 200 }}>
      <MoneyInput
        currency="EUR"
        valueMinor={minor}
        onChangeMinor={setMinor}
        display={(amount) => formatAnalyticsMoney(amount, "EUR", locale)}
        aria-label="Monthly goal"
      />
    </div>
  );
}

/** At rest the amount reads as the page writes money; focus shows the number to type. */
export const WithDisplayForm: Story = {
  render: () => <DisplayDemo />,
};

/**
 * To a German reader "1.500" is fifteen hundred; to the decimal rule it is one
 * and a half. The box keeps the text and names the wanted form.
 */
export const RefusesAnAmbiguousAmount: Story = {
  render: () => (
    <LocaleProvider initial="de">
      <MoneyDemo />
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const box = await canvas.findByLabelText("Unit price");
    await userEvent.clear(box);
    await userEvent.type(box, "1.500");
    await userEvent.tab();
    await expect(
      await canvas.findByText("Gib den Betrag als 1500 oder 1500,00 ein."),
    ).toBeTruthy();
  },
};
