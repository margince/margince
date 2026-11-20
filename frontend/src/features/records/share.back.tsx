// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RecordType } from "../../components/share.parts";
import { RecordBack } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { recordRoute } from "../../lib/entity";
import { routeHash } from "../../lib/router";

/** The way back from the Sharing page to the record it shares. */
export function ShareBack({
  recordType,
  recordId,
  name,
}: Readonly<{ recordType: RecordType; recordId: string; name: string }>) {
  const t = useT();
  const route = recordRoute(recordType, recordId);
  if (!route) {
    return null;
  }
  return (
    <RecordBack
      href={routeHash(route)}
      label={t("shell.navBackTo", { name })}
    />
  );
}
