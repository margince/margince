// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Checkbox } from "../../design-system/atoms";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

export function InternalMailHistoryChoice({
  offer,
  checked,
  disabled,
  onChange,
}: Readonly<{
  offer: components["schemas"]["InternalMailHistoryPreview"] | undefined;
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  if (!offer) return null;
  return (
    <section className="form-stack" aria-label={t("internalMail.historyTitle")}>
      <p>
        {t("internalMail.historyAfter", {
          date: formatDateTime(offer.received_after, locale, zone),
        })}
      </p>
      <p>
        {t("internalMail.historyFolders")}:{" "}
        {offer.folders.length
          ? offer.folders.join(", ")
          : t("internalMail.allFolders")}
      </p>
      <p>{t("internalMail.historyImportNote")}</p>
      <Checkbox
        label={t("internalMail.includeHistory")}
        checked={checked}
        disabled={disabled}
        onChange={() => onChange(!checked)}
      />
    </section>
  );
}
