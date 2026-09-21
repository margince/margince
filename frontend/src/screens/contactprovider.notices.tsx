// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What the bought-contact-data panel says about ITSELF, above the values it
// holds: that a run is still moving. Not content, and not the refusal a POST
// answered with either — that is the shared `WriteRefused`, and a run the
// PLATFORM declined is neither, being a skipped run the state badge names.

/**
 * A run that is still moving, above the values rather than in place of them.
 *
 * A lookup takes about half a minute, and for that half-minute the panel used
 * to be identical to one where nothing had happened: no line, no change, the
 * same buttons. A reader who pressed and saw nothing move concluded the button
 * was broken, which was the only conclusion available to them.
 *
 * `asking` is the half where the provider has not answered yet; the other half
 * has the answer and is writing it onto the record.
 */
export function LookupRunning({
  asking,
  provider,
}: Readonly<{ asking: boolean; provider: string }>) {
  const t = useT();
  return (
    <Callout
      kind="event"
      title={t(
        asking
          ? "provider.profile.workingTitle"
          : "provider.profile.landingTitle",
        { provider },
      )}
    >
      {asking ? t("provider.profile.working") : t("provider.profile.landing")}
    </Callout>
  );
}
