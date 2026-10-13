import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Sparkles, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import {
  Button,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { Select } from "../design-system/select";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import {
  REVIEW_TEMPLATES_KEY,
  type ReviewQuestion,
  type ReviewTemplate,
} from "./outcomereview.queries";
import { ReviewTemplateSummary } from "./reviewtemplatesummary";
import "./reviewtemplates.css";

// A template edit affects future submissions. The server versions the change;
// saved reviews keep their questions, options and answers as they were asked.
export function ReviewTemplateEditor({
  template,
  onClose,
}: Readonly<{
  template: ReviewTemplate;
  onClose: () => void;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const titleId = useId();
  const [questions, setQuestions] = useState<ReviewQuestion[]>(
    template.questions,
  );
  const save = useMutation({
    mutationFn: async (submitted: {
      id: string;
      version: number;
      questions: ReviewQuestion[];
    }) => {
      const { data, error, response } = await api.PATCH(
        "/activity-review-templates/{id}",
        {
          params: { path: { id: submitted.id } },
          body: { version: submitted.version, questions: submitted.questions },
        },
      );
      if (error || !response.ok) throwProblem(error);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: REVIEW_TEMPLATES_KEY });
      onClose();
    },
  });
  function change(index: number, update: Partial<ReviewQuestion>) {
    setQuestions((current) =>
      current.map((question, i) =>
        i === index ? { ...question, ...update } : question,
      ),
    );
  }
  // Order is meaning here: the close reason fills whichever question is first.
  function move(index: number, by: -1 | 1) {
    setQuestions((current) => {
      const next = [...current];
      const [moved] = next.splice(index, 1);
      next.splice(index + by, 0, moved);
      return next;
    });
  }
  const invalid =
    questions.length === 0 ||
    questions.some(
      (q) =>
        !q.label.trim() ||
        (q.type === "multiselect" &&
          !(q.options ?? []).some((option) => option.trim())),
    );
  const dirty =
    JSON.stringify(questions) !== JSON.stringify(template.questions);
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
      size="wide"
    >
      <div className="drawer-head">
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {template.label}
        </Heading>
        <p className="t-caption">
          {t(
            template.outcome === "won"
              ? "reviewTemplates.drawer.won"
              : "reviewTemplates.drawer.lost",
          )}
        </p>
      </div>
      <div className="drawer-body">
        <Callout tone="info" title={t("reviewTemplates.prefillCallout.title")}>
          {t("reviewTemplates.prefillCallout.body")}
        </Callout>
        <ol className="rt-questions">
          {questions.map((question, index) => (
            <QuestionCard
              key={question.key}
              question={question}
              index={index}
              count={questions.length}
              disabled={save.isPending}
              onChange={(update) => change(index, update)}
              onMove={(by) => move(index, by)}
              onRemove={() =>
                setQuestions((current) =>
                  current.filter((q) => q.key !== question.key),
                )
              }
            />
          ))}
        </ol>
        <Button
          variant="ghost"
          disabled={save.isPending}
          onClick={() =>
            setQuestions((current) => [
              ...current,
              {
                key: `question_${crypto.randomUUID()}`,
                label: "",
                type: "text",
                required: false,
              },
            ])
          }
        >
          {t("reviewTemplates.addQuestion")}
        </Button>
        <ErrorLine error={save.error} />
      </div>
      <div className="drawer-foot rt-foot">
        <ReviewTemplateSummary questions={questions} />
        {dirty && (
          <span className="t-caption">{t("reviewTemplates.unsaved")}</span>
        )}
        <span className="rt-foot-actions">
          <Button variant="ghost" disabled={save.isPending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
            variant="primary"
            pending={save.isPending}
            disabled={invalid}
            onClick={() =>
              save.mutate({
                id: template.id,
                version: template.version,
                questions: questions.map((q) => ({
                  ...q,
                  options:
                    q.type === "multiselect"
                      ? q.options
                          ?.map((option) => option.trim())
                          .filter(Boolean)
                      : undefined,
                })),
              })
            }
          >
            {t("reviewTemplates.saveQuestions")}
          </Button>
        </span>
      </div>
    </Modal>
  );
}

function QuestionCard({
  question,
  index,
  count,
  disabled,
  onChange,
  onMove,
  onRemove,
}: Readonly<{
  question: ReviewQuestion;
  index: number;
  count: number;
  disabled: boolean;
  onChange: (update: Partial<ReviewQuestion>) => void;
  onMove: (by: -1 | 1) => void;
  onRemove: () => void;
}>) {
  const t = useT();
  const n = { n: String(index + 1) };
  return (
    <li className="rt-q">
      <span className="t-label rt-q-num" aria-hidden="true">
        {index + 1}
      </span>
      <div className="form-stack">
        <Field label={t("reviewTemplates.question")} required>
          {(control) => (
            <TextInput
              {...control}
              value={question.label}
              disabled={disabled}
              onChange={(event) => onChange({ label: event.target.value })}
            />
          )}
        </Field>
        {index === 0 && (
          <span className="t-caption rt-prefill">
            <Sparkles aria-hidden="true" />
            {t("reviewTemplates.prefilled")}
          </span>
        )}
        <div className="rt-q-opts">
          <Field label={t("reviewTemplates.col.answer")}>
            {(control) => (
              <Select
                {...control}
                value={question.type}
                disabled={disabled}
                options={[
                  { value: "text", label: t("reviewTemplates.type.text") },
                  {
                    value: "multiselect",
                    label: t("reviewTemplates.type.multiselect"),
                  },
                ]}
                onChange={(value) =>
                  onChange({
                    type: value === "multiselect" ? "multiselect" : "text",
                    options: [],
                  })
                }
              />
            )}
          </Field>
          <Switch
            label={t("reviewTemplates.requiredBadge")}
            checked={question.required}
            disabled={disabled}
            onChange={(required) => onChange({ required })}
          />
        </div>
        {question.type === "multiselect" && (
          <Field
            label={t("reviewTemplates.choices")}
            hint={t("reviewTemplates.choicesHint")}
            required
          >
            {(control) => (
              <Textarea
                {...control}
                rows={4}
                disabled={disabled}
                value={(question.options ?? []).join("\n")}
                onChange={(event) =>
                  onChange({ options: event.target.value.split("\n") })
                }
              />
            )}
          </Field>
        )}
      </div>
      <div className="rt-q-tools">
        {index > 0 && (
          <IconAction
            label={t("reviewTemplates.moveUp", n)}
            icon={<ArrowUp />}
            disabled={disabled}
            onClick={() => onMove(-1)}
          />
        )}
        {index < count - 1 && (
          <IconAction
            label={t("reviewTemplates.moveDown", n)}
            icon={<ArrowDown />}
            disabled={disabled}
            onClick={() => onMove(1)}
          />
        )}
        <IconAction
          label={t("reviewTemplates.removeNumbered", n)}
          icon={<Trash2 />}
          disabled={disabled || count === 1}
          onClick={onRemove}
        />
      </div>
    </li>
  );
}
