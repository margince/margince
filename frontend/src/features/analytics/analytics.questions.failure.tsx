// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { labelFor } from "../../lib/labelfor";
import { ProblemError } from "../../lib/problem";
import {
  REFUSAL_KEY,
  refusalOf,
  refusalWords,
} from "./analytics.questions.vocab";

/**
 * Why a question was not answered. A refusal is guidance, not a fault. The
 * heading says in the reader's language what kind of refusal it is. The
 * engine's own account follows, with the smallest change that would have
 * worked first. Anything else is an ordinary failure, in the reader's words.
 */
export function QuestionFailure({
  error,
  onRetry,
}: Readonly<{ error: unknown; onRetry?: () => void }>) {
  const t = useT();
  const refusal =
    error instanceof ProblemError ? refusalOf(error.problem) : null;
  if (!refusal) {
    return (
      <ErrorLine
        error={error}
        actions={
          onRetry ? (
            <Button onClick={onRetry}>{t("common.retry")}</Button>
          ) : undefined
        }
      />
    );
  }
  const { message, suggest } = refusalWords(t, refusal);
  return (
    <Callout
      tone="info"
      kind="outcome"
      title={t(labelFor(REFUSAL_KEY, refusal.kind))}
    >
      {suggest !== "" && <p>{suggest}</p>}
      {message !== "" && <p>{message}</p>}
    </Callout>
  );
}
