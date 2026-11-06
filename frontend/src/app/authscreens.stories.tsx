import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import {
  AuthScreen,
  AvailabilityScreen,
  ReleaseSkewScreen,
  SetupClaimScreen,
} from "./authscreens";

const meta: Meta = {
  title: "Signed out/Boundary loaders",
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  beforeEach: () =>
    installFetchStub({
      "GET /auth/capabilities": () =>
        jsonResponse({
          password: true,
          password_reset: true,
          oidc_providers: [],
        }),
    }),
};
export default meta;
type Story = StoryObj;
export const SignIn: Story = {
  render: () => <AuthScreen onAuthed={() => undefined} />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: "Sign in",
    });
  },
};
export const SignInDark: Story = { ...SignIn, globals: { theme: "dark" } };
export const Connection: Story = {
  beforeEach: () =>
    installFetchStub({
      "GET /me": () => {
        throw new TypeError("Failed to fetch");
      },
    }),
  render: () => (
    <AvailabilityScreen kind="connection" onRetry={() => undefined} />
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", { name: "Retry" });
  },
};
export const ConnectionDark: Story = {
  ...Connection,
  globals: { theme: "dark" },
};
export const Setup: Story = {
  render: () => (
    <SetupClaimScreen
      onClaimed={() => undefined}
      status={{ claimable: true, jurisdictions: ["de", "vn"] }}
    />
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByLabelText(/Company name/);
  },
};
export const SetupDark: Story = { ...Setup, globals: { theme: "dark" } };

export const ReleaseSkew: Story = {
  render: () => <ReleaseSkewScreen app="2026.1" server="2026.2" />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", { name: /Reload/ });
  },
};
export const ReleaseSkewDark: Story = {
  ...ReleaseSkew,
  globals: { theme: "dark" },
};
