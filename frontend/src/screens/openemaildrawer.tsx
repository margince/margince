// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EmailDetail } from "../design-system/emaildetail";
import { formatDateTime } from "../format/format";
import { useLocale } from "../i18n";

/**
 * The record page's one email drawer.
 *
 * A component rather than a conditional at each call site: five record pages
 * mount the same three lines, and `{openEmail && <EmailDetail …/>}` spends a
 * branch inside render callbacks that are already at the complexity ceiling.
 * Nothing is drawn when no message is open.
 */
export function OpenEmailDrawer({
  activityId,
  zone,
  onClose,
}: Readonly<{
  activityId: string | null;
  /** The record's timezone, which the page owns. */
  zone: string;
  onClose: () => void;
}>) {
  const { locale } = useLocale();
  if (!activityId) {
    return null;
  }
  return (
    <EmailDetail
      activityId={activityId}
      onClose={onClose}
      formatWhen={(iso) => formatDateTime(iso, locale, zone)}
    />
  );
}
