// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { ActionRow } from "../../design-system/actionrow";
import {
  Badge,
  Button,
  Card,
  Checkbox,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { IconAction } from "../../design-system/iconaction";
import { type FieldFaults, useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { ordinalNumber } from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { blankRequired, throwProblem } from "../../lib/problem";
import { shippedLabel } from "../../lib/shippedlabels";
import {
  REVIEW_TEMPLATES_KEY,
  type ReviewQuestion,
  type ReviewTemplate,
} from "./outcomereview.queries";
import { ReviewTemplateSummary } from "./reviewtemplatesummary";
import "./reviewtemplates.css";

type Outcome = ReviewTemplate["outcome"];

type Submitted =
  | { id: string; version: number; questions: ReviewQuestion[] }
  | { label: string; outcome: Outcome; questions: ReviewQuestion[] };

// The question a new template starts with, so the drawer never opens empty.
function blankQuestion(): ReviewQuestion {
  return {
    key: `question_${crypto.randomUUID()}`,
    label: "",
    type: "text",
    required: false,
  };
}

async function submit(submitted: Submitted) {
  if ("id" in submitted) {
    const { data, error, response } = await api.PATCH(
      "/activity-review-templates/{id}",
      {
        params: { path: { id: submitted.id } },
        body: { version: submitted.version, questions: submitted.questions },
      },
    );
    if (error || !response.ok) throwProblem(error);
    return data;
  }
  const { data, error, response } = await api.POST(
    "/activity-review-templates",
    { body: submitted },
  );
  if (error || !response.ok) throwProblem(error);
  return data;
}

// A template edit affects future submissions. The server versions the change;
// saved reviews keep their questions, options and answers as they were asked.
// With no template the drawer adds one, which the server makes the one its
// outcome asks from then on.
export function ReviewTemplateEditor({
  template,
  onClose,
}: Readonly<{
  template?: ReviewTemplate;
  onClose: () => void;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const titleId = useId();
  // The editor shows a built-in question in the reader's language, as it was
  // when the drawer opened. A language switch then neither edits nor rewords it.
  const [shown] = useState(() =>
    template
      ? template.questions.map((question) => ({
          ...question,
          label: shippedLabel(question, t),
        }))
      : [blankQuestion()],
  );
  const [questions, setQuestions] = useState<ReviewQuestion[]>(shown);
  const [label, setLabel] = useState("");
  const [outcome, setOutcome] = useState<Outcome>("won");
  const save = useMutation({
    mutationFn: submit,
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
      if (moved) next.splice(index + by, 0, moved);
      return next;
    });
  }
  const { locale } = useLocale();
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const dirty = JSON.stringify(questions) !== JSON.stringify(shown);
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {template ? shippedLabel(template, t) : t("reviewTemplates.newTitle")}
        </Heading>
        <p className="t-caption">
          {t(
            (template?.outcome ?? outcome) === "won"
              ? "reviewTemplates.drawer.won"
              : "reviewTemplates.drawer.lost",
          )}
        </p>
      </DrawerHead>
      <DrawerBody ref={faultsRoot}>
        {!template && (
          <div className="form-stack">
            <Field
              label={t("reviewTemplates.name")}
              required
              error={faults.of("label")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  value={label}
                  maxLength={120}
                  disabled={save.isPending}
                  onChange={(event) => {
                    setLabel(event.target.value);
                    faults.edited("label");
                  }}
                />
              )}
            </Field>
            <Field label={t("reviewTemplates.askedFor")}>
              {(control) => (
                <Select
                  {...control}
                  value={outcome}
                  disabled={save.isPending}
                  options={[
                    {
                      value: "won",
                      label: t("reviewTemplates.askedFor.won"),
                    },
                    {
                      value: "lost",
                      label: t("reviewTemplates.askedFor.lost"),
                    },
                  ]}
                  onChange={(value) =>
                    setOutcome(value === "lost" ? "lost" : "won")
                  }
                />
              )}
            </Field>
          </div>
        )}
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
              faults={faults}
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
            setQuestions((current) => [...current, blankQuestion()])
          }
        >
          {t("reviewTemplates.addQuestion")}
        </Button>
        <ErrorLine error={save.error} />
      </DrawerBody>
      <EditorFoot
        questions={questions}
        dirty={dirty}
        pending={save.isPending}
        onCancel={onClose}
        saveLabel={t(
          template ? "reviewTemplates.saveQuestions" : "reviewTemplates.add",
        )}
        onSave={() => {
          const blank = templateFaults(
            locale,
            t,
            template ? undefined : label,
            questions,
          );
          if (faults.show(blank)) return;
          save.mutate(
            template
              ? {
                  id: template.id,
                  version: template.version,
                  questions: questions
                    .map((question) => storedWording(question, shown, template))
                    .map(trimmedChoices),
                }
              : {
                  label: label.trim(),
                  outcome,
                  questions: questions.map(trimmedChoices),
                },
          );
        }}
      />
    </Modal>
  );
}

// The name of a new template, each question's wording, and a list
// question's choices, keyed as the fields below key their sentences. The
// choices label reads as an instruction, so its sentence goes unnamed.
function templateFaults(
  locale: Locale,
  t: Translator,
  label: string | undefined,
  questions: readonly ReviewQuestion[],
): Map<string, string> {
  const choices = (q: ReviewQuestion) =>
    q.type === "multiselect" ? (q.options ?? []).join("").trim() : "listed";
  return blankRequired(locale, {
    ...(label === undefined
      ? {}
      : { label: [t("reviewTemplates.name"), label] as const }),
    ...Object.fromEntries(
      questions.flatMap((q) => [
        [`q-${q.key}`, [t("reviewTemplates.question"), q.label] as const],
        [`opts-${q.key}`, [undefined, choices(q)] as const],
      ]),
    ),
  });
}

// An untouched built-in question sends the wording it is stored with, so the
// server keeps its catalog key and other readers keep their own language.
function storedWording(
  question: ReviewQuestion,
  shown: readonly ReviewQuestion[],
  template: ReviewTemplate,
): ReviewQuestion {
  const asShown = shown.find((q) => q.key === question.key);
  const stored = template.questions.find((q) => q.key === question.key);
  return asShown && stored?.label_key && question.label === asShown.label
    ? { ...question, label: stored.label }
    : question;
}

// A list question sends its non-blank choices; a free-text one sends none.
function trimmedChoices(question: ReviewQuestion): ReviewQuestion {
  return {
    ...question,
    options:
      question.type === "multiselect"
        ? question.options?.map((option) => option.trim()).filter(Boolean)
        : undefined,
  };
}

// The drawer's verbs, laid out like the page save bar. The count and the
// unsaved caption lead, Cancel sits beside them, the one primary Save ends.
function EditorFoot({
  questions,
  dirty,
  pending,
  saveLabel,
  onCancel,
  onSave,
}: Readonly<{
  questions: readonly ReviewQuestion[];
  dirty: boolean;
  pending: boolean;
  saveLabel: string;
  onCancel: () => void;
  onSave: () => void;
}>) {
  const t = useT();
  return (
    <DrawerFoot>
      <ActionRow
        className="rt-foot"
        primary={
          <Button variant="primary" pending={pending} onClick={onSave}>
            {saveLabel}
          </Button>
        }
      >
        <ReviewTemplateSummary questions={questions} />
        {dirty && (
          <span className="t-caption">{t("reviewTemplates.unsaved")}</span>
        )}
        <Button variant="ghost" disabled={pending} onClick={onCancel}>
          {t("deals.cancel")}
        </Button>
      </ActionRow>
    </DrawerFoot>
  );
}

function QuestionCard({
  question,
  index,
  count,
  disabled,
  faults,
  onChange,
  onMove,
  onRemove,
}: Readonly<{
  question: ReviewQuestion;
  index: number;
  count: number;
  disabled: boolean;
  faults: FieldFaults;
  onChange: (update: Partial<ReviewQuestion>) => void;
  onMove: (by: -1 | 1) => void;
  onRemove: () => void;
}>) {
  const t = useT();
  const n = { n: String(index + 1) };
  return (
    <Card as="li" inset className="rt-q">
      <span className="t-label rt-q-num" aria-hidden="true">
        {ordinalNumber(index + 1)}
      </span>
      <div className="form-stack">
        <Field
          label={t("reviewTemplates.question")}
          required
          error={faults.of(`q-${question.key}`)}
        >
          {(control) => (
            <TextInput
              {...control}
              value={question.label}
              disabled={disabled}
              onChange={(event) => {
                faults.edited(`q-${question.key}`);
                onChange({ label: event.target.value });
              }}
            />
          )}
        </Field>
        {index === 0 && (
          <span>
            <Badge tone="ai" wrap>
              {t("reviewTemplates.prefilled")}
            </Badge>
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
                onChange={(value) => {
                  faults.edited(`opts-${question.key}`);
                  onChange({
                    type: value === "multiselect" ? "multiselect" : "text",
                    options: [],
                  });
                }}
              />
            )}
          </Field>
          {/* Staged with the drawer's Save, so a box rather than a switch. */}
          <Checkbox
            label={t("reviewTemplates.requiredBadge")}
            checked={question.required}
            disabled={disabled}
            onChange={(event) => onChange({ required: event.target.checked })}
          />
        </div>
        {question.type === "multiselect" && (
          <Field
            label={t("reviewTemplates.choices")}
            hint={t("reviewTemplates.choicesHint")}
            required
            error={faults.of(`opts-${question.key}`)}
          >
            {(control) => (
              <Textarea
                {...control}
                rows={4}
                disabled={disabled}
                value={(question.options ?? []).join("\n")}
                onChange={(event) => {
                  faults.edited(`opts-${question.key}`);
                  onChange({ options: event.target.value.split("\n") });
                }}
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
    </Card>
  );
}
