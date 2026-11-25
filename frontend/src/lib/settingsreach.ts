// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { unitsForSecretScope } from "./extensions";
import { useMe } from "./session";
import {
  type CatalogContext,
  type SettingsReach,
  settingsReach,
} from "./settingscatalog";

// Which extension units this build composed. Read here rather than inside the
// catalog: the extensions registry reaches `@composition/screens`, and
// importing it there would drag React into a module importable from anywhere.
export function catalogContext(): CatalogContext {
  return {
    composedUnitScopes: (["workspace", "user"] as const).filter(
      (scope) => unitsForSecretScope(scope).length > 0,
    ),
  };
}

/**
 * The settings pages, split into the ones this reader can act on and the ones
 * they can only consult.
 *
 * The hook half of `settingsReach`, so the rail, the settings home, the setup
 * reminder and Home resolve one partition rather than each deciding "can this
 * contact act?" for itself.
 */
export function useSettingsReach(): SettingsReach {
  return settingsReach(useMe().data, catalogContext());
}
