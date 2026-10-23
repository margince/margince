// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { RecordTabs } from "../../design-system/recordtabs";
import { useT } from "../../i18n";

// A project is read in ONE body, so the strip names that one and nothing else.
const PROJECT_TABS = ["overview"] as const;

/** ProjectTabs is the project record's tab row. */
export function ProjectTabs() {
  const t = useT();

  return (
    <RecordTabs
      options={PROJECT_TABS}
      value="overview"
      // No handler: with one body the strip draws a label rather than a tab to
      // press, so there is no click to route.
      labels={{ overview: t("tab.overview") }}
    />
  );
}
