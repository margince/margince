// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronDown, FileText } from "lucide-react";
import { Button } from "../../design-system/atoms";
import { Popover } from "../../design-system/popover";
import { useT } from "../../i18n";
import {
  openAnalyticsSection,
  type Section,
} from "../../lib/analytics.address";
import { navigate } from "../../lib/router";
import { SETUP_LABEL, type SetupSection } from "./analytics.tabs";

/** The verbs at the end of the tab row: the saved reports and the Setup menu. */
export function TabVerbs({
  library,
  setup,
  section,
}: Readonly<{
  library: Section | undefined;
  setup: readonly SetupSection[];
  section: Section;
}>) {
  const t = useT();
  if (!library && setup.length === 0) return null;
  return (
    <div className="analytics-tab-verbs">
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
        <Popover
          variant="ghost"
          label={
            <>
              {t("analytics.setup")}
              <ChevronDown aria-hidden="true" />
            </>
          }
        >
          {setup.map((candidate) => (
            <Button
              key={candidate}
              variant="link"
              onClick={() => openAnalyticsSection(candidate)}
            >
              {t(SETUP_LABEL[candidate])}
            </Button>
          ))}
          {/* Revenue goals are set on their own page; whoever reads targets
              reads them. */}
          {setup.includes("targets") && (
            <Button
              variant="link"
              onClick={() => navigate({ screen: "goals" })}
            >
              {t("nav.salesGoals")}
            </Button>
          )}
        </Popover>
      )}
    </div>
  );
}
