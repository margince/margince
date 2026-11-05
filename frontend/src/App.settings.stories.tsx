// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Suspense } from "react";
import { expect, within } from "storybook/test";
import { SettingsScreen as SettingsRoute } from "./App.screens";
import { SettingsScreen } from "./App.settings";
import { runningDataTask } from "./features/settings/datataskshealth.fixtures";
import { settingsAddress } from "./features/settings/settings";
import { healthReport } from "./features/settings/systemhealth.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./testing/story-utils";

const meta = {
  title: "Shell/Settings",
  component: SettingsScreen,
} satisfies Meta<typeof SettingsScreen>;
export default meta;
type Story = StoryObj;

function installReview() {
  installFetchStub({
    "GET /me": meRoute({
      data_task: ["read", "update"],
      activity: ["read", "update", "delete"],
    }),
    "GET /admin/system-health": () =>
      jsonResponse({
        ...healthReport,
        sources: healthReport.sources.map((source) =>
          source.id === "updates"
            ? source
            : {
                ...source,
                availability: "withheld",
                condition: "unknown",
                code: "withheld",
              },
        ),
      }),
    "GET /admin/data-tasks": () =>
      jsonResponse({
        data: [
          {
            ...runningDataTask,
            title: "Review automatic work",
            review_kind: "home_attention",
            status: "pending",
            blocked_reason: "review_required",
          },
        ],
      }),
  });
}

export const ReviewedWork: Story = {
  render: () => {
    installReview();
    return (
      <StoryProviders>
        <Suspense fallback={null}>
          <SettingsScreen
            route={{ ...settingsAddress("system-health"), id2: "diagnostics" }}
          />
        </Suspense>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "Preview items",
      }),
    ).toBeInTheDocument();
  },
};

export const LazyRoute: Story = {
  render: () => {
    installReview();
    return (
      <StoryProviders>
        <Suspense fallback={null}>
          <SettingsRoute
            route={{ ...settingsAddress("system-health"), id2: "diagnostics" }}
          />
        </Suspense>
      </StoryProviders>
    );
  },
};
