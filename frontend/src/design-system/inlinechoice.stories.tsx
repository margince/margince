import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { InlineText } from "./inlinechoice";

const meta: Meta = {
  title: "Design System/Inline text editing",
  decorators: [
    (Story) => (
      <LocaleProvider>
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;
function Example({ multiline = false }: { multiline?: boolean }) {
  const [value, setValue] = useState(
    multiline ? "A paragraph with intentional whitespace." : "14.60",
  );
  return (
    <InlineText
      label={multiline ? "Brief" : "Value"}
      placeholder="Not set"
      value={value}
      multiline={multiline}
      type={multiline ? "text" : "number"}
      step="any"
      canEdit
      onSave={async (next) => setValue(next)}
    />
  );
}
export const FractionalNumber: Story = { render: () => <Example /> };
export const Paragraph: Story = { render: () => <Example multiline /> };
