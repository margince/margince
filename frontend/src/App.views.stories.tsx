// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { ScreenView } from "./App.views";
import { routeHash } from "./lib/router";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./testing/story-utils";

const meta = {
  title: "Shell/Route content",
  component: ScreenView,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ScreenView>;
export default meta;
type Story = StoryObj<typeof ScreenView>;

export const Onboarding: Story = {
  render: () => {
    globalThis.location.hash = routeHash({ screen: "onboarding" });
    installFetchStub({
      "GET /me": meRoute({}, { roles: ["admin"], seat: "full" }),
      "GET /company/context/capabilities": () =>
        jsonResponse({
          rollout: "off",
          read_enabled: false,
          tasks_enabled: false,
          onboarding_enabled: false,
        }),
      "GET /company": () => jsonResponse({}, 404),
      "GET /onboarding/state": () => jsonResponse({}, 404),
      "GET /voice-profiles": () => jsonResponse({ data: [], page: {} }),
    });
    return (
      <StoryProviders>
        <ScreenView screen="onboarding" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const name = await within(canvasElement).findByLabelText("Company name");
    await waitFor(() => expect(name).toBeVisible());
  },
};

export const OnboardingDark: Story = {
  ...Onboarding,
  globals: { theme: "dark" },
};
