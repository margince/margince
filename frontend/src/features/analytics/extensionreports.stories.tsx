// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ExtensionReports } from "./extensionreports";

const meta: Meta<typeof ExtensionReports> = {
  title: "Records/Reports/Extension reports",
  component: ExtensionReports,
};
export default meta;
type Story = StoryObj<typeof ExtensionReports>;

export const RegisteredCounts: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /extension-reports": () =>
          jsonResponse({
            data: [
              {
                unit: "demo",
                population: "ext:demo:visit",
                name: "ext:demo:visits",
                label: "Visits",
                kind: "count",
                status: "ok",
                withheld: false,
                total: 13,
                groups: [{ key: null, count: 13, withheld: false }],
                excluded_by_permission: null,
              },
            ],
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <ExtensionReports />
      </StoryProviders>
    );
  },
};

export const Withheld: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /extension-reports": () =>
          jsonResponse({
            data: [
              {
                unit: "demo",
                population: "ext:demo:visit",
                name: "ext:demo:visits",
                label: "Visits",
                kind: "count",
                status: "ok",
                withheld: true,
                total: null,
                groups: [{ key: null, count: null, withheld: true }],
                excluded_by_permission: null,
              },
            ],
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <ExtensionReports />
      </StoryProviders>
    );
  },
};

export const OneUnavailable: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /extension-reports": () =>
          jsonResponse({
            data: [
              {
                unit: "demo",
                population: "ext:demo:visit",
                name: "ext:demo:by_site",
                label: "Visits by site",
                kind: "breakdown",
                by: "site",
                status: "unavailable",
                withheld: false,
                total: null,
                groups: [],
                excluded_by_permission: null,
              },
              {
                unit: "demo",
                population: "ext:demo:visit",
                name: "ext:demo:visits",
                label: "Visits",
                kind: "count",
                status: "ok",
                withheld: false,
                total: 13,
                groups: [{ key: null, count: 13, withheld: false }],
                excluded_by_permission: null,
              },
            ],
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <ExtensionReports />
      </StoryProviders>
    );
  },
};
