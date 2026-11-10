// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { type CoreFeature, screenEnabled } from "../lib/corefeatures";
import { navigateReplacing } from "../lib/router";
import { firstAvailableRoute } from "./nav";

export function useFeatureLanding(
  ready: boolean,
  confined: boolean,
  disabled: readonly CoreFeature[],
) {
  useEffect(() => {
    if (
      ready &&
      !confined &&
      ["", "#", "#/"].includes(window.location.hash) &&
      !screenEnabled("home", disabled)
    ) {
      navigateReplacing(firstAvailableRoute(disabled));
    }
  }, [ready, confined, disabled]);
}
