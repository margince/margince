// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { useMe } from "./session";

/**
 * The company's name for a sentence that reads "everyone at {company}", or
 * "your company" while the session read is still out. The German phrase is
 * dative ("bei deinem Unternehmen"), so a sentence opening on it cannot use it.
 */
export function useCompanyPhrase(): string {
  const t = useT();
  return useMe().data?.workspace_name || t("ob.conv.connect.yourCompany");
}
