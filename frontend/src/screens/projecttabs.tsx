// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PageAsideToggle } from "../app/pageaside";
import { RecordTabs } from "../design-system/recordtabs";
import { useT } from "../i18n";

// A project is read in ONE body, so the strip names that one and nothing else.
// What earns a strip with no choice in it is the control riding at its end: the
// details switch belongs to the row that chooses what the work column shows,
// and every other record page carries it there, so a reader who has learned
// where the switch lives finds it in the same place on a project.
const PROJECT_TABS = ["overview"] as const;

/** ProjectTabs is the project record's tab row, carrying the details switch. */
export function ProjectTabs() {
  const t = useT();

  return (
    <RecordTabs
      options={PROJECT_TABS}
      value="overview"
      // No handler: with one body the strip draws a label rather than a tab to
      // press, so there is no click to route.
      labels={{ overview: t("tab.overview") }}
      trailing={<PageAsideToggle />}
    />
  );
}
