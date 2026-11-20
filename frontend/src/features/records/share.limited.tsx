// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useT } from "../../i18n";

/**
 * The colleague a write share was last saved for as read, because of their
 * role. On such a save the notice replaces "Nothing changed", which may be false.
 */
export function useLimitedByRole() {
  const [name, setName] = useState<string | null>(null);
  return {
    name,
    note(answer: { limited_by_role?: boolean } | undefined, subject: string) {
      const limited = answer?.limited_by_role === true;
      setName(limited ? subject : null);
      return limited;
    },
    clear: () => setName(null),
  };
}

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
