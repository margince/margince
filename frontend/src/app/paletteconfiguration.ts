// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The palette's arm for a setting's own name: a key or variable an operator
// holds from the docs opens Configuration filtered to it.

import {
  CONFIGURATION_FILTER_PARAM,
  useConfiguration,
} from "../features/settings/configurationentries";
import {
  CONFIGURATION_PAGE,
  configurationFilterFor,
} from "../features/settings/settingssearch";
import type { SettingsPageId } from "../lib/settingscatalog";
import type { Command } from "./palette";

/** The id of the command that opens one settings page. */
export function settingsCommandId(page: SettingsPageId): string {
  return `screen:settings-${page}`;
}

/**
 * Configuration, opened on the settings the query names by key or variable.
 *
 * Offered only where `commands` holds the page, which is where the reader may
 * open it, and only when the page's own words did not already match.
 */
export function useConfigurationCommand(
  query: string,
  commands: readonly Command[],
  matched: readonly Command[],
): Command | null {
  const page = commands.find(
    (command) => command.id === settingsCommandId(CONFIGURATION_PAGE),
  );
  const configuration = useConfiguration(
    page !== undefined && query.trim() !== "",
  );
  if (page === undefined || matched.includes(page)) {
    return null;
  }
  const setting = configurationFilterFor(query, configuration.data ?? []);
  if (setting === undefined) {
    return null;
  }
  return {
    ...page,
    id: `${page.id}:${CONFIGURATION_FILTER_PARAM}`,
    subtitle: setting.named,
    dials: new Map([[CONFIGURATION_FILTER_PARAM, setting.filter]]),
  };
}
