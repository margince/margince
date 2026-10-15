// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { openActivityImport } from "../../screens/importaddress";

/**
 * Starts an import of activities that all belong to this contact. Shown only
 * to a reader who may run an import. It asks the same two grants as the import
 * page, so the verb never leads to a page that refuses them.
 */
export function ImportActivitiesAction({
  contactId,
  reasonId,
}: Readonly<{ contactId: string; reasonId?: string }>) {
  const t = useT();
  const mayCreate = useCanWrite("import_run", "create");
  const mayAdvance = useCan("import_run", "update");
  const mayImport = mayCreate && mayAdvance;
  if (!mayImport) return null;
  return (
    <Button reasonId={reasonId} onClick={() => openActivityImport(contactId)}>
      {t("contact.action.importActivities")}
    </Button>
  );
}
