// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import { contactQuery } from "../../lib/contact.queries";

/**
 * Where a file of activities goes, said before the reader chooses anything.
 * The file has no links column to show it, so the contact is named here.
 */
export function ImportPresetTarget({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const contact = useQuery(contactQuery(contactId));
  const name = contact.data?.full_name;
  return (
    <Callout
      tone="info"
      kind="standing"
      title={
        name
          ? t("import.preset.into", { name })
          : t("import.preset.intoUnnamed")
      }
    >
      {t("import.preset.hint")}
    </Callout>
  );
}
