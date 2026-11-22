// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Certification = components["schemas"]["AiTaskCertification"];

// The certification grade of the model that answers a task. The server reads it
// from the committed records, and it is the grade the test results page prints.
const GRADES: Readonly<
  Record<Certification["grade"], { kind: StatusKind; label: MessageKey }>
> = {
  ready: { kind: "done", label: "aiCert.ready" },
  usable_with_care: { kind: "attention", label: "aiCert.withCare" },
  not_reliable: { kind: "failure", label: "aiCert.notReliable" },
  not_measured: { kind: "neutral", label: "aiCert.notMeasured" },
};

export function CertificationBadge({
  certification,
}: Readonly<{ certification: Certification | undefined }>) {
  const t = useT();
  if (!certification) {
    return null;
  }
  // A grade a newer server adds reads as not measured here, never as ready.
  const grade = Object.hasOwn(GRADES, certification.grade)
    ? GRADES[certification.grade]
    : GRADES.not_measured;
  const label = t(grade.label);
  return (
    <span title={t("aiCert.title", { model: certification.model })}>
      <StatusBadge
        kind={grade.kind}
        label={
          certification.recheck_pending
            ? t("aiCert.recheckPending", { grade: label })
            : label
        }
      />
    </span>
  );
}
