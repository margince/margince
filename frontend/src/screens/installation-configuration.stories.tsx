// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { InstallationConfigurationPage } from "./installation-configuration";

const ENTRIES = [
  {
    key: "database.url",
    env: "MARGINCE_DATABASE_URL",
    type: "string",
    doc: "The connection string of the database.",
    roles: ["api", "worker", "migrate"],
    secret: true,
    stored: false,
    value: null,
    default: null,
    origin: { source: "env_file", name: "MARGINCE_DATABASE_URL", file: ".env" },
  },
  {
    key: "http.port",
    env: "MARGINCE_HTTP_PORT",
    flag: "http-port",
    type: "int",
    doc: "The port the api listens on.",
    roles: ["api"],
    secret: false,
    stored: false,
    value: "8080",
    default: "8080",
    origin: { source: "default", name: "" },
  },
  // A struct value is JSON with no spaces: the row that once held the table
  // wider than its panel.
  {
    key: "ai.budget",
    env: "MARGINCE_AI_BUDGET",
    type: "struct",
    doc: "The AI token allowance per seat and per company.",
    roles: ["api", "worker"],
    secret: false,
    stored: true,
    value:
      '{"tokens_per_full_user":12000000,"company_monthly_tokens":null,"tokens_per_light_user":3000000}',
    default: null,
    origin: { source: "env", name: "MARGINCE_AI_BUDGET" },
  },
  // The longest key and variable name the registry holds, set from the
  // environment so the source repeats the name.
  {
    key: "installation.follow_up_reconcile_interval_seconds",
    env: "MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS",
    type: "int",
    doc: "How often the follow-up reconcile pass runs.",
    roles: ["worker"],
    secret: false,
    stored: true,
    value: "900",
    default: "900",
    origin: {
      source: "env",
      name: "MARGINCE_INSTALLATION_FOLLOW_UP_RECONCILE_INTERVAL_SECONDS",
    },
  },
  {
    key: "worker.concurrency",
    env: "MARGINCE_WORKER_CONCURRENCY",
    type: "int",
    doc: "How many jobs the worker runs at once.",
    roles: ["worker"],
    secret: false,
    stored: false,
    value: null,
    default: null,
    origin: { source: "unset", name: "" },
  },
];

function page() {
  installFetchStub({
    "GET /me": meRoute({ installation_settings: ["read", "update"] }),
    "GET /installation/configuration": () => jsonResponse(ENTRIES),
  });
  return (
    <StoryProviders>
      <InstallationConfigurationPage />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/Governance/Configuration/Page",
  component: InstallationConfigurationPage,
} satisfies Meta<typeof InstallationConfigurationPage>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: page,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("http.port");
    await expect(canvas.getByText("Secret")).toBeInTheDocument();
    await expect(canvas.getByText("Not set")).toBeInTheDocument();
  },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  render: page,
};
