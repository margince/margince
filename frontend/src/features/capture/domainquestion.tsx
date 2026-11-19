import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import type { MessageKey } from "../../i18n/en";
import { ProblemError, problemCodeOf } from "../../lib/problem";
import { problemDetails } from "../../lib/problembody";

// A 409 the server sends when a value is already something on file. The same
// request, sent again with `confirm: true`, stores it.
const QUESTION_CODE = "domain_needs_confirmation";

export type DomainQuestionScope = "consumerList" | "ownAddress";

type Question = Readonly<{ role: string; company: string | null }>;

export function domainQuestionOf(error: unknown): Question | null {
  if (
    !(error instanceof ProblemError) ||
    problemCodeOf(error) !== QUESTION_CODE
  ) {
    return null;
  }
  const details = problemDetails(error.problem);
  const company = details?.company_name;
  return {
    role: String(details?.role ?? ""),
    company: typeof company === "string" && company !== "" ? company : null,
  };
}

function questionKey(
  scope: DomainQuestionScope,
  question: Question,
): MessageKey {
  if (scope === "consumerList") {
    return "captureValue.askConsumerList";
  }
  return question.role === "contact_address"
    ? "captureValue.askContactAddress"
    : "captureValue.askCompanyDomain";
}

/** Asks whether a value already on file should be stored anyway. */
export function DomainQuestionCallout({
  error,
  value,
  scope,
  pending,
  onConfirm,
  t,
}: Readonly<{
  error: unknown;
  value: string;
  scope: DomainQuestionScope;
  pending: boolean;
  onConfirm: () => void;
  t: (key: MessageKey, params?: Record<string, string>) => string;
}>) {
  const question = domainQuestionOf(error);
  if (question === null) {
    return null;
  }
  return (
    <Callout tone="warning" kind="outcome" title={t("captureValue.askTitle")}>
      <p>
        {t(questionKey(scope, question), {
          value,
          company: question.company ?? t("captureValue.aCompany"),
        })}
      </p>
      <Button type="button" disabled={pending} onClick={onConfirm}>
        {t("captureValue.addAnyway")}
      </Button>
    </Callout>
  );
}
