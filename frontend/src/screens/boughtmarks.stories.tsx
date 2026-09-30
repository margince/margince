import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ContactLink } from "../design-system/contactlink";
import { BoughtMark, BoughtText } from "./boughtmarks";
import { StoryProviders } from "./story-utils";

// The two shapes a bought value takes: plain text underlined in place, and a
// value that is already a control, with the mark beside it rather than inside.

const bought = {
  target: "phone:ph-1",
  provider: "surfe",
  applied_at: "2026-06-02T12:00:00Z",
};

function Marks() {
  return (
    <StoryProviders>
      <p>
        <BoughtText value="Head of Revenue" bought={bought} />
      </p>
      <p className="fieldgrid-handle">
        <ContactLink kind="phone" value="+4915112345678" />
        <BoughtMark bought={bought} subject="+4915112345678" />
      </p>
      {/* Not bought: the same value with nothing behind it draws no mark. */}
      <p className="fieldgrid-handle">
        <ContactLink kind="phone" value="+4930123456" />
        <BoughtMark bought={undefined} subject="+4930123456" />
      </p>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Contact 360/Bought values",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const Resting: Story = { render: () => <Marks /> };

export const ReceiptOpen: Story = {
  render: () => <Marks />,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: "bought: where “+4915112345678” came from",
      }),
    );
  },
};

export const ReceiptOpenDark: Story = {
  ...ReceiptOpen,
  globals: { theme: "dark" },
};
