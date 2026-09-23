import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import {
  Button,
  Checkbox,
  Field,
  Modal,
  Textarea,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import {
  REVIEW_TEMPLATES_KEY,
  type ReviewQuestion,
  type ReviewTemplate,
} from "./outcomereview.queries";

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
  const invalid =
    questions.length === 0 ||
    questions.some(
      (q) =>
        !q.label.trim() ||
        (q.type === "multiselect" &&
          !(q.options ?? []).some((option) => option.trim())),
    );
  return (
    <Modal open onClose={onClose} labelledBy="review-template-heading">
      <Heading size="large" id="review-template-heading" className="t-h2">
        {template.label}
      </Heading>
      <p>{t("reviewTemplates.editHint")}</p>
      <div className="form-stack">
        {questions.map((question, index) => (
          <div key={question.key} className="form-stack">
            <Field label={t("reviewTemplates.question")} required>
              {(control) => (
                <TextInput
                  {...control}
                  value={question.label}
                  disabled={save.isPending}
                  onChange={(event) =>
                    change(index, { label: event.target.value })
                  }
                />
              )}
            </Field>
            <Field label={t("reviewTemplates.answerType")}>
              {(control) => (
                <Select
                  {...control}
                  value={question.type}
                  disabled={save.isPending}
                  options={[
                    { value: "text", label: t("cf.type.text") },
                    { value: "multiselect", label: t("cf.type.multiselect") },
                  ]}
                  onChange={(value) =>
                    change(index, {
                      type: value === "multiselect" ? "multiselect" : "text",
                      options: [],
                    })
                  }
                />
              )}
            </Field>
            {question.type === "multiselect" && (
              <Field label={t("reviewTemplates.options")} required>
                {(control) => (
                  <Textarea
                    {...control}
                    rows={5}
                    disabled={save.isPending}
                    value={(question.options ?? []).join("\n")}
                    onChange={(event) =>
                      change(index, { options: event.target.value.split("\n") })
                    }
                  />
                )}
              </Field>
            )}
            <Checkbox
              label={t("reviewTemplates.requiredChoice")}
              checked={question.required}
              disabled={save.isPending}
              onChange={() => change(index, { required: !question.required })}
            />
            <Button
              variant="ghost"
              disabled={save.isPending || questions.length === 1}
              onClick={() =>
                setQuestions((current) =>
                  current.filter((q) => q.key !== question.key),
                )
              }
            >
              {t("reviewTemplates.removeQuestion")}
            </Button>
          </div>
        ))}
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
        <div className="actions">
          <Button variant="ghost" disabled={save.isPending} onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button
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
            {t("reviewTemplates.save")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
