// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { usePlural, useT } from "../i18n";
import type { ReviewQuestion } from "./outcomereview.queries";

// "3 questions, 1 required": the settings panel head and the editor's foot
// count the same list, so they read it through this one line.
export function ReviewTemplateSummary({
  questions,
}: Readonly<{ questions: readonly ReviewQuestion[] }>) {
  const t = useT();
  const plural = usePlural();
  const required = questions.filter((question) => question.required).length;
  return (
    <span className="t-caption">
      {t("reviewTemplates.summary", {
        questions: plural("reviewTemplates.questionCount", questions.length, {
          count: String(questions.length),
        }),
        required: String(required),
      })}
    </span>
  );
}
