// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { GrantSpec } from "../app/mefixture";
import type { Route } from "../app/router";
import { PageTitle, SettingsRail } from "../app/shell";
import { en } from "../i18n/en";
import { SETTINGS_SCREEN, useSettingsSection } from "./settingsnav";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  WithInstallationBrand,
} from "./story-utils";
import "./settings.css";

// The sidebar's settings level as the shell mounts it, built by
// `useSettingsSection` from the session each story serves on `GET /me`. The
// shell stories hold the level still as a fixture; these are the live grants.

type Session = Readonly<{
  allow: GrantSpec;
  roles: string[];
}>;

// An admin holding the company's settings, so every audience group stands.
const ADMIN: Session = {
  roles: ["admin"],
  allow: {
    installation_settings: ["read", "create", "update"],
    user_admin: ["read", "create", "update"],
    team_admin: ["read", "create", "update"],
    role_admin: ["read", "create", "update"],
    pipeline: ["read", "create", "update"],
    tag: ["read", "create", "update"],
    custom_field: ["read", "create", "update"],
    product: ["read", "create", "update"],
    automation: ["read", "create", "update"],
    capture_settings: ["read", "create", "update"],
    integrations: ["read", "create", "update"],
    ai_routing: ["read", "create", "update"],
    ai_budget: ["read", "create", "update"],
    retention_policy: ["read", "create", "update"],
    privacy_request: ["read", "create", "update"],
    ai_diagnostics: ["read"],
    job_health: ["read"],
    license: ["read"],
  },
};

// A rep with no object grant: the personal pages are the ones no grant gates.
const MEMBER: Session = { roles: ["rep"], allow: {} };

// Fields is a page this seat may read and not change, so the rail leaves it
// out until the reader opens it.
const READS_FIELDS: Session = {
  roles: ["manager"],
  allow: { pipeline: ["read", "create", "update"], custom_field: ["read"] },
};

function stubSession(session: Session) {
  installFetchStub({
    "GET /me": meRoute(session.allow, { roles: session.roles, seat: "full" }),
    "GET /ai/usage": () =>
      jsonResponse({
        days: [],
        budget: { monthly_tokens: 100, spent_tokens: 20, band: "normal" },
      }),
  });
}

const BRAND = { display_name: "Gradion GmbH" };

function PhoneFrame({ route }: Readonly<{ route: Route }>) {
  const section = useSettingsSection(route);
  return (
    <div className="app">
      <SettingsRail route={route} />
      <main className="main">
        <PageTitle route={route} section={section} />
        <div className="scroll" />
      </main>
    </div>
  );
}

function rail(session: Session) {
  return ({ route }: Readonly<{ route: Route }>) => {
    stubSession(session);
    return (
      <StoryProviders>
        <WithInstallationBrand brand={BRAND}>
          <div className="app railexpanded">
            <SettingsRail route={route} />
            <main className="main">
              <div className="scroll" />
            </main>
          </div>
        </WithInstallationBrand>
      </StoryProviders>
    );
  };
}

function phone(session: Session) {
  return ({ route }: Readonly<{ route: Route }>) => {
    stubSession(session);
    return (
      <StoryProviders>
        <WithInstallationBrand brand={BRAND}>
          <PhoneFrame route={route} />
        </WithInstallationBrand>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Settings navigation",
  component: SettingsRail,
  parameters: { layout: "fullscreen" },
  args: { route: { screen: SETTINGS_SCREEN, id: "account" } },
} satisfies Meta<typeof SettingsRail>;
export default meta;
type Story = StoryObj<typeof meta>;

export const AdminOnPersonalPage: Story = { render: rail(ADMIN) };

export const AdminOnCompanyPage: Story = {
  args: { route: { screen: SETTINGS_SCREEN, id: "pipelines" } },
  render: rail(ADMIN),
};

export const AdminOnHome: Story = {
  args: { route: { screen: SETTINGS_SCREEN } },
  render: rail(ADMIN),
};

export const AdminDark: Story = {
  args: { route: { screen: SETTINGS_SCREEN, id: "pipelines" } },
  globals: { theme: "dark" },
  render: rail(ADMIN),
};

export const Member: Story = { render: rail(MEMBER) };

export const OpenedOffRail: Story = {
  args: { route: { screen: SETTINGS_SCREEN, id: "fields" } },
  render: rail(READS_FIELDS),
};

// At phone width the level leaves the bar and the page heading becomes its
// switcher.
export const Phone: Story = {
  args: { route: { screen: SETTINGS_SCREEN, id: "pipelines" } },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: phone(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: en["shell.sectionSwitch"].replace(
          "{name}",
          en["settings.tab.pipelines"],
        ),
      }),
    );
  },
};
