import { ArrowLeft, ArrowRight } from "lucide-react";
import { useEffect, useState } from "react";
import { Button, Textarea, TextInput } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { ordinalNumber } from "../format/format";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type CompanyFieldName,
  type CompanyForm,
  isMultilineField,
  isRequired,
} from "./onboarding";

// The manual company interview: one question at a time, legal identity
// first, required questions gating advance and optional ones skippable.
// The conversational shell hosts it in the artifact panel when the human
// would rather tell than have their website read.

type ManualQuestion = Readonly<{
  field: Exclude<CompanyFieldName, "website">;
  chapter: MessageKey;
  prompt: MessageKey;
  hint: MessageKey;
}>;

const MANUAL_QUESTIONS: readonly ManualQuestion[] = [
  {
    field: "legal_name",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.legal_name",
    hint: "ob.manual.legal_nameHint",
  },
  {
    field: "registered_address",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.registered_address",
    hint: "ob.manual.registered_addressHint",
  },
  {
    field: "register_vat",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.register_vat",
    hint: "ob.manual.register_vatHint",
  },
  {
    field: "legal_form",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.legal_form",
    hint: "ob.manual.legal_formHint",
  },
  {
    field: "register_court",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.register_court",
    hint: "ob.manual.register_courtHint",
  },
  {
    field: "register_number",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.register_number",
    hint: "ob.manual.register_numberHint",
  },
  {
    field: "display_name",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.display_name",
    hint: "ob.manual.display_nameHint",
  },
  {
    field: "offer_summary",
    chapter: "ob.manualChapterOffer",
    prompt: "ob.manual.offer_summary",
    hint: "ob.manual.offer_summaryHint",
  },
  {
    field: "icp",
    chapter: "ob.manualChapterCustomer",
    prompt: "ob.manual.icp",
    hint: "ob.manual.icpHint",
  },
  {
    field: "industry",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.industry",
    hint: "ob.manual.industryHint",
  },
  {
    field: "history",
    chapter: "ob.manualChapterLegal",
    prompt: "ob.manual.history",
    hint: "ob.manual.historyHint",
  },
  {
    field: "value_proposition",
    chapter: "ob.manualChapterOffer",
    prompt: "ob.manual.value_proposition",
    hint: "ob.manual.value_propositionHint",
  },
  {
    field: "usp",
    chapter: "ob.manualChapterOffer",
    prompt: "ob.manual.usp",
    hint: "ob.manual.uspHint",
  },
  {
    field: "buying_center",
    chapter: "ob.manualChapterCustomer",
    prompt: "ob.manual.buying_center",
    hint: "ob.manual.buying_centerHint",
  },
  {
    field: "customer_pains",
    chapter: "ob.manualChapterCustomer",
    prompt: "ob.manual.customer_pains",
    hint: "ob.manual.customer_painsHint",
  },
  {
    field: "desired_outcomes",
    chapter: "ob.manualChapterCustomer",
    prompt: "ob.manual.desired_outcomes",
    hint: "ob.manual.desired_outcomesHint",
  },
  {
    field: "buying_intents",
    chapter: "ob.manualChapterSales",
    prompt: "ob.manual.buying_intents",
    hint: "ob.manual.buying_intentsHint",
  },
  {
    field: "common_objections",
    chapter: "ob.manualChapterSales",
    prompt: "ob.manual.common_objections",
    hint: "ob.manual.common_objectionsHint",
  },
  {
    field: "sales_motion",
    chapter: "ob.manualChapterSales",
    prompt: "ob.manual.sales_motion",
    hint: "ob.manual.sales_motionHint",
  },
];

export function ManualCompanyInterview({
  values,
  setField,
  onPersist,
  onBackToChoice,
  onComplete,
}: Readonly<{
  values: CompanyForm;
  setField: (field: CompanyFieldName, value: string) => void;
  onPersist: () => void;
  onBackToChoice: () => void;
  onComplete: () => void;
}>) {
  const t = useT();
  const [questionIndex, setQuestionIndex] = useState(0);
  const question = MANUAL_QUESTIONS[questionIndex];
  const answerID = `manual-question-${question?.field ?? "unavailable"}-answer`;

  useEffect(() => {
    document.getElementById(answerID)?.focus();
  }, [answerID]);

  if (!question) {
    return null;
  }
  const required = isRequired(question.field);
  const value = values[question.field];
  const last = questionIndex === MANUAL_QUESTIONS.length - 1;
  const advance = () => {
    if (required && value.trim() === "") {
      return;
    }
    onPersist();
    if (last) {
      onComplete();
      return;
    }
    setQuestionIndex((current) => current + 1);
  };
  const back = () => {
    onPersist();
    if (questionIndex === 0) {
      onBackToChoice();
      return;
    }
    setQuestionIndex((current) => current - 1);
  };
  const promptID = `manual-question-${question.field}`;
  const hintID = `${promptID}-hint`;
  // The answer box is the design system's, but NOT inside a `Field`: this board
  // asks one question at a time and that question is its <h1>, so a `Field`
  // would draw a second name for the same control — the same words a rung
  // smaller, in the ink a hint wears. The heading names it and the line under
  // the heading describes it, which is what the two aria attributes say.
  const answer = {
    id: answerID,
    "aria-labelledby": promptID,
    "aria-describedby": hintID,
    value,
    required,
    onChange: (event: { target: { value: string } }) =>
      setField(question.field, event.target.value),
    onBlur: onPersist,
  };

  return (
    <form
      className="ob-core-dialog ob-manual-question"
      onSubmit={(event) => {
        event.preventDefault();
        advance();
      }}
    >
      <div className="ob-manual-progress">
        <span>{t(question.chapter)}</span>
        <span>
          {ordinalNumber(questionIndex + 1)} /{" "}
          {ordinalNumber(MANUAL_QUESTIONS.length)}
        </span>
      </div>
      <Heading size="xlarge" id={promptID}>
        {t(question.prompt)}
      </Heading>
      <p id={hintID}>{t(question.hint)}</p>
      {isMultilineField(question.field) ? (
        // Three lines open, because these questions ask for a paragraph and a
        // box the height of a line says a sentence is what is wanted.
        <Textarea rows={3} {...answer} />
      ) : (
        <TextInput {...answer} />
      )}
      <div className="ob-manual-actions">
        <Button variant="link" type="button" onClick={back}>
          <ArrowLeft aria-hidden /> {t("ob.back")}
        </Button>
        <Button
          variant="primary"
          type="submit"
          disabled={required && value.trim() === ""}
        >
          {last
            ? t("ob.manualReview")
            : !required && value.trim() === ""
              ? t("ob.manualLater")
              : t("ob.manualNext")}
          <ArrowRight aria-hidden />
        </Button>
      </div>
      <small className="ob-manual-required t-caption">
        {required ? t("ob.manualRequired") : t("ob.manualOptional")}
      </small>
    </form>
  );
}
