/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { COMPANY_CONTEXT_ON } from "../features/settings/settingspages.fixtures";
import { settingsSearch } from "../features/settings/settingssearch";
import { LocaleProvider, translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { RbacAction } from "../lib/accessgrants";
import { companyContextCapabilitiesQueryKey } from "../lib/companycontextreach";
import {
  type CapabilityExpression,
  SETTINGS_PAGES,
  type SettingsPageId,
} from "../lib/settingscatalog";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { type Command, matchingCommands, useBuiltinCommands } from "./palette";

afterEach(cleanup);

// What a reader types for a page, one thought per phrase. Several are words the
// product once printed and no longer does, so a reader who learned them still lands.
const PHRASES: readonly (readonly [string, SettingsPageId])[] = [
  ["password", "account"],
  ["profile", "account"],
  ["working hours", "meetings"],
  ["writing style", "voice"],
  ["tone", "voice"],
  ["api key", "agents"],
  ["passport", "agents"],
  ["digest", "notifications"],
  ["mailbox", "connections"],
  ["capture log", "capture-activity"],
  ["fx", "company"],
  ["fiscal year", "company"],
  ["oauth app", "authentication"],
  ["sign-in", "authentication"],
  ["sso", "authentication"],
  ["invite", "members"],
  ["license", "seats"],
  ["deal stages", "pipelines"],
  ["lead sources", "leads"],
  ["custom-fields", "fields"],
  ["data model", "fields"],
  ["labels", "tags"],
  ["price list", "products"],
  ["rate card", "products"],
  ["offer-templates", "products"],
  ["email capture", "capture"],
  ["webhook", "integrations"],
  ["handbook", "knowledge"],
  ["csv", "import"],
  ["embeddings", "models"],
  ["triggers", "automations"],
  ["spend", "usage"],
  ["logs", "model-calls"],
  ["gdpr", "privacy"],
  ["law", "jurisdiction"],
  ["trail", "audit"],
  ["reindex", "system-health"],
  ["plugins", "extensions"],
  ["delete everything", "reset"],
  // The name of one setting on the page, which no alias carries.
  ["data retention", "system-health"],
  ["date format", "company"],
  ["sync interval", "capture"],
  ["two-step", "account"],
];

// Seats and license cannot change what anyone pays.
const BILLING_WORDS = ["billing", "plan", "subscription"];

// Every grant the catalog asks for, so every page is open to this reader.
function everyCatalogGrant(): GrantSpec {
  const allow: Record<string, RbacAction[]> = {};
  const walk = (expression: CapabilityExpression) => {
    if (expression.kind === "grant") {
      allow[expression.object] = [
        ...(allow[expression.object] ?? []),
        expression.action,
      ];
    }
    if (expression.kind === "any" || expression.kind === "all") {
      expression.of.forEach(walk);
    }
  };
  for (const page of SETTINGS_PAGES) {
    walk(page.requires);
    walk(page.changes);
  }
  return allow;
}

async function adminCommands(): Promise<readonly Command[]> {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity } },
  });
  client.setQueryData(["me"], {
    ...meFixture({ roles: ["admin"], allow: everyCatalogGrant() }),
    data_reset_available: true,
  });
  client.setQueryData(companyContextCapabilitiesQueryKey, COMPANY_CONTEXT_ON);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const view = renderHook(() => useBuiltinCommands(), { wrapper });
  await waitFor(() =>
    expect(
      view.result.current.filter((command) =>
        command.id.startsWith("screen:settings-"),
      ),
    ).toHaveLength(SETTINGS_PAGES.length),
  );
  return view.result.current;
}

const t = (key: MessageKey) => translate("en", key);

function paletteFinds(commands: readonly Command[], phrase: string) {
  return matchingCommands(commands, phrase).map((command) => command.id);
}

function searchFinds(phrase: string): SettingsPageId[] {
  return settingsSearch(phrase, SETTINGS_PAGES, t, {
    seesCompanyContext: true,
  }).map((hit) => hit.page.id);
}

describe("the palette and the settings search", () => {
  it.each(PHRASES)("both send %j to %s", async (phrase, page) => {
    const commands = await adminCommands();
    expect(paletteFinds(commands, phrase)).toContain(`screen:settings-${page}`);
    expect(searchFinds(phrase)).toContain(page);
  });

  it.each(BILLING_WORDS)(
    "neither sends %j to Seats and license",
    async (word) => {
      const commands = await adminCommands();
      expect(paletteFinds(commands, word)).not.toContain(
        "screen:settings-seats",
      );
      expect(searchFinds(word)).not.toContain("seats");
    },
  );
});

describe("the palette's matcher", () => {
  const company: Command = {
    id: "screen:settings-company",
    label: "Hồ sơ công ty",
    keywords: ["Múi giờ báo cáo", "Khoảng đồng bộ (giây)"],
    type: "screen",
  };

  it.each(["mui gio bao cao", "bao cao gio", "dong bo", "ho so cong"])(
    "finds %p through every word and without its marks",
    (query) => {
      expect(matchingCommands([company], query)).toEqual([company]);
    },
  );

  it("drops a command that holds only some of the words", () => {
    expect(matchingCommands([company], "mui kubernetes")).toEqual([]);
  });
});
