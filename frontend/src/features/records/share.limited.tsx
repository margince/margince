// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../../i18n";

/**
 * Says that an edit share was saved as read, because the colleague's role
 * cannot edit this kind of record. A null record means the company's contacts.
 */
export function LimitedByRoleNotice({
  name,
  record,
}: Readonly<{ name: string | null; record: string | null }>) {
  const t = useT();
  if (name === null) {
    return null;
  }
  return (
    <p role="status" data-testid="share-limited-by-role">
      {record === null
        ? t("share.companyContactsLimitedByRole", { name })
        : t("share.limitedByRole", { name, record })}
    </p>
  );
}
