// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CoreFeature } from "./corefeaturepolicy";
import { useMe } from "./session";

export {
  type CoreFeature,
  recordEnabled,
  screenEnabled,
} from "./corefeaturepolicy";

const NONE: readonly CoreFeature[] = [];

export function useDisabledCoreFeatures(): readonly CoreFeature[] {
  return useMe(false).data?.settings_availability?.disabled_features ?? NONE;
}
