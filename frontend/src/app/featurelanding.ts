// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { type CoreFeature, screenEnabled } from "../lib/corefeatures";
import { navigateReplacing, useHash } from "../lib/router";
import { firstAvailableRoute } from "./nav";

export function useFeatureLanding(
  ready: boolean,
  confined: boolean,
  disabled: readonly CoreFeature[],
) {
  const hash = useHash();
  useEffect(() => {
    if (
      ready &&
      !confined &&
      ["", "#", "#/"].includes(hash) &&
      !screenEnabled("home", disabled)
    ) {
      navigateReplacing(firstAvailableRoute(disabled));
    }
  }, [ready, confined, disabled, hash]);
}
