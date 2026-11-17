// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import { type CoreFeature, screenEnabled } from "../lib/corefeatures";
import { useHash } from "../lib/router";
import { useAddressWriters } from "../lib/urlstate";
import { firstAvailableRoute } from "./nav";

export function useFeatureLanding(
  ready: boolean,
  confined: boolean,
  disabled: readonly CoreFeature[],
) {
  const hash = useHash();
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    if (
      ready &&
      !confined &&
      ["", "#", "#/"].includes(hash) &&
      !screenEnabled("home", disabled)
    ) {
      navigateReplacing(firstAvailableRoute(disabled));
    }
  }, [ready, confined, disabled, hash, navigateReplacing]);
}
