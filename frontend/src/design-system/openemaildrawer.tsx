// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatDateTime } from "../format/format";
import { useLocale } from "../i18n";
import { EmailAccessEditor } from "../screens/emailaccesseditor";
import { EmailDetail } from "./emaildetail";

/**
 * The record page's one email drawer.
 *
 * A component rather than a conditional at each call site: five record pages
 * mount the same three lines, and `{openEmail && <EmailDetail …/>}` spends a
 * branch inside render callbacks that are already at the complexity ceiling.
 * Nothing is drawn when no message is open.
 *
 * This is also where the access editor is bound, so every page that mounts the
 * drawer gets it. `EmailDetail` takes it as a render prop and never imports it:
 * the editor performs writes and reads the roster, which is screen work, and
 * the catalog component stays something a story can draw with no API behind it.
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
      renderAccess={(presentation) => (
        <EmailAccessEditor presentation={presentation} />
      )}
    />
  );
}
