// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { BusyMark } from "../design-system/atoms";
import { useWaited } from "../design-system/waited";
import { useT } from "../i18n";

// Most starts answer inside this delay, and then draw no splash at all. Both
// gates in AuthedApp return this in one place, so the delay runs on through both.
const AUTH_SPLASH_DELAY_MS = 400;

export function AuthSplash() {
  const t = useT();
  const waited = useWaited(AUTH_SPLASH_DELAY_MS);
  if (!waited) {
    return null;
  }
  // The words are for a screen reader only: on screen, the turning mark says
  // that the app is starting, and "session" names an internal.
  return (
    <div className="boot-splash" role="status">
      <BusyMark />
      <span className="sr-only">{t("auth.checking")}</span>
    </div>
  );
}
