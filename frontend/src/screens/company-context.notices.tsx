// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What the company-context card says about ITSELF: the confirmation a save
// leaves behind, and the caveats a website read came back with. Its four
// refusals — the cold-start form, the profile dialog, the site-read panel and
// the review's Apply — are the shared `WriteRefused` in `common.tsx`, which is
// where the same nine lines went once every card's copy of them was one.

/**
 * The save landed, and the dialog it landed in is gone — so the confirmation
 * is left on the card that now shows the new values. A heading alone is the
 * whole notice; there is nothing to add to "Saved".
 */
export function SavedNotice() {
  const t = useT();
  return (
    <Callout tone="success" kind="outcome" title={t("settings.companySaved")} />
  );
}

/**
 * The caveats one website read came back with, a line each under ONE heading.
 *
 * A callout per warning made a five-page read look like five separate things
 * going wrong. Empty renders nothing, so the caller states no condition of its
 * own: whether there is anything to say is this notice's question.
 */
export function ReadWarnings({
  warnings,
}: Readonly<{ warnings: readonly string[] }>) {
  const t = useT();
  if (warnings.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="warn"
      kind="standing"
      title={t("settings.companyRefreshWarnings")}
    >
      <ul>
        {warnings.map((warning) => (
          <li key={warning}>{warning}</li>
        ))}
      </ul>
    </Callout>
  );
}
