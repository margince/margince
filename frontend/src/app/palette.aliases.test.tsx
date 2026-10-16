/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import type { RbacAction } from "../lib/accessgrants";
import {
  type CapabilityExpression,
  SETTINGS_PAGES,
} from "../screens/settingscatalog";
import { settingsPageAliases } from "../screens/settingssearch";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { useBuiltinCommands } from "./palette";

afterEach(cleanup);

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

function commandsFor(roles: string[]) {
  const client = new QueryClient();
  client.setQueryData(["me"], {
    ...meFixture({ roles, allow: everyCatalogGrant() }),
    data_reset_available: true,
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return renderHook(() => useBuiltinCommands(), { wrapper });
}

describe("the palette and the settings search", () => {
  it.each([
    ["an admin", ["admin"], true],
    ["a rep", ["rep"], false],
  ] as const)(
    "give %s the same words for every page",
    async (_, roles, admin) => {
      const view = commandsFor([...roles]);
      const settingsKeywords = () =>
        new Map(
          view.result.current
            .filter((command) => command.id.startsWith("screen:settings-"))
            .map((command) => [command.id, command.keywords ?? []]),
        );
      await waitFor(() =>
        expect(settingsKeywords().size).toBe(SETTINGS_PAGES.length),
      );
      for (const page of SETTINGS_PAGES) {
        const keywords = settingsKeywords().get(`screen:settings-${page.id}`);
        const extra =
          page.id === "company" && admin
            ? [translate("en", "settings.companyRefresh")]
            : [];
        expect(keywords).toEqual([
          page.id,
          ...settingsPageAliases(page.id, { holdsAdminRole: admin }),
          ...extra,
        ]);
      }
    },
  );

  // Seats and license cannot change what anyone pays.
  it("sends neither search to Seats and license for billing words", () => {
    for (const word of ["billing", "plan", "subscription"]) {
      expect(settingsPageAliases("seats")).not.toContain(word);
    }
  });
});
