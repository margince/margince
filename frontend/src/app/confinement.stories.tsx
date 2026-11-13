import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { INTRO_MS } from "../design-system/motion";
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
  // The button arrives with the page's intro, which takes longer than the
  // default wait.
  await waitFor(() => expect(button).toBeVisible(), {
    timeout: INTRO_MS + 1000,
  });
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
