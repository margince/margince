import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { StoryProviders } from "../testing/story-utils";
import { ConfinedAccountScreen } from "./confinement";

const meta: Meta<typeof ConfinedAccountScreen> = {
  title: "Signed out/Account confinement",
  component: ConfinedAccountScreen,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: { kind: "must-enrol-mfa", onResolved: () => undefined },
};
export default meta;
type Story = StoryObj<typeof ConfinedAccountScreen>;
async function ready({ canvasElement }: { canvasElement: HTMLElement }) {
  const button = await within(canvasElement).findByRole("button", {
    name: "Set up",
  });
  await waitFor(() => expect(button).toBeVisible());
}
export const MfaRequired: Story = { play: ready };
export const MfaRequiredDark: Story = {
  globals: { theme: "dark" },
  play: ready,
};

export const PasswordRequired: Story = {
  args: { kind: "must-change-password" },
};
export const PasswordRequiredDark: Story = {
  args: { kind: "must-change-password" },
  globals: { theme: "dark" },
};
