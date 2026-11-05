// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import type { ReportingEvaluation } from "./reporting.model";
import { WrittenSummary } from "./reporting.written";

/**
 * The assessment card: the position in a few sentences, written from the
 * figures the report already carries. It adds no figure of its own, so a
 * reader who cannot see a figure does not read it here either.
 */
export function AssessmentCard({
  evaluation,
  wide,
}: Readonly<{ evaluation: ReportingEvaluation; wide: boolean }>) {
  const t = useT();
  return (
    <Panel
      title={t("reporting.assessmentCard")}
      className={
        wide ? "reporting-panel reporting-panel-wide" : "reporting-panel"
      }
    >
      <PanelBody>
        <WrittenSummary evaluation={evaluation} />
        <p className="t-caption">{t("reporting.assessmentDeterministic")}</p>
      </PanelBody>
    </Panel>
  );
}
