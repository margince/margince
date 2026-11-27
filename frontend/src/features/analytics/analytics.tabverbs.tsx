// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown, FileText } from "lucide-react";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { createPortalSlot } from "../../design-system/portalslot";
import { useT } from "../../i18n";
import {
  openAnalyticsSection,
  type Section,
} from "../../lib/analytics.address";
import { navigate } from "../../lib/router";
import { SETUP_LABEL, type SetupSection } from "./analytics.tabs";

/** Where a section's own View menu stands, first among the tab row's verbs. */
export const AnalyticsViewSlot = createPortalSlot();

/** The verbs at the end of the tab row: the section's View, the saved reports
 * and the Setup menu. */
export function TabVerbs({
  library,
  setup,
  section,
  viewSlot,
}: Readonly<{
  library: Section | undefined;
  setup: readonly SetupSection[];
  section: Section;
  viewSlot: (element: HTMLElement | null) => void;
}>) {
  const t = useT();
  return (
    <div className="analytics-tab-verbs">
      <span className="analytics-view-slot" ref={viewSlot} />
      {library && (
        <Button
          variant="ghost"
          aria-current={section === library ? "page" : undefined}
          onClick={() => openAnalyticsSection(library)}
        >
          <FileText aria-hidden="true" />
          {t(
            library === "reports"
              ? "analytics.sectionReports"
              : "analytics.sectionQuestions",
          )}
        </Button>
      )}
      {setup.length > 0 && (
        <OverflowMenu
          label={t("analytics.setup")}
          text={t("analytics.setup")}
          face="button"
          icon={<ChevronDown aria-hidden="true" />}
        >
          {setup.map((candidate) => (
            <Button
              key={candidate}
              onClick={() => openAnalyticsSection(candidate)}
            >
              {t(SETUP_LABEL[candidate])}
            </Button>
          ))}
          {/* Revenue goals are set on their own page; whoever reads targets
              reads them. */}
          {setup.includes("targets") && (
            <Button onClick={() => navigate({ screen: "goals" })}>
              {t("nav.salesGoals")}
            </Button>
          )}
        </OverflowMenu>
      )}
    </div>
  );
}
