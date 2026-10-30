import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { formatDate } from "../../format/format.dates";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  savedQuestionLabel,
  savedQuestionShape,
  useSavedQuestions,
} from "./analytics.questions.saved";
import { customLabels, entityLabel } from "./analytics.questions.vocab";
import { useAnalyticsSchema } from "./analytics.schema";
import {
  cardQuestion,
  layoutEdits as edit,
  MAX_CARDS,
  type ReportingLayout,
  runIdOf,
} from "./reporting.layout";

type Card = ReportingLayout[number]["cards"][number];

/**
 * Adds a saved custom question: one of the reader's own from a list, or any
 * other from its link. The card is bound to the question when the report is
 * saved, filters included. The server keeps the filters, so no reader of the
 * report receives them. A later save of another question does not move a
 * published report.
 */
export function AddQuestion({
  layout,
  onChange,
}: Readonly<{
  layout: ReportingLayout;
  onChange: (next: ReportingLayout) => void;
}>) {
  const t = useT();
  const [link, setLink] = useState("");
  const runId = runIdOf(link);
  const add = (run: string, asked: Parameters<typeof cardQuestion>[0]) =>
    onChange(
      edit.addQuestion(layout, {
        ...cardQuestion(asked),
        saved_question_id: run,
      }),
    );
  const read = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.GET("/analytics/runs/{run_id}", {
          params: { path: { run_id: id } },
        }),
      ),
    onSuccess: (run) => {
      add(run.id, run.query);
      setLink("");
    },
  });
  if (layout.flatMap((section) => section.cards).length >= MAX_CARDS)
    return null;
  return (
    <div className="form-stack">
      <SavedQuestionPicker layout={layout} onPick={add} />
      <Field
        label={t("reporting.questionLink")}
        hint={link && !runId ? t("reporting.questionLinkInvalid") : undefined}
      >
        {(field) => (
          <TextInput
            {...field}
            value={link}
            onChange={(event) => setLink(event.target.value)}
          />
        )}
      </Field>
      <ErrorLine error={read.error} />
      <div>
        <Button
          disabled={!runId}
          pending={read.isPending}
          onClick={() => runId && read.mutate(runId)}
        >
          {t("reporting.addQuestion")}
        </Button>
      </div>
    </div>
  );
}

/**
 * The author's own saved questions, read a page at a time once the author asks
 * for them, about one population when one is chosen. A row names its question
 * and asks nothing.
 */
function SavedQuestionPicker({
  layout,
  onPick,
}: Readonly<{
  layout: ReportingLayout;
  onPick: (run: string, asked: Parameters<typeof cardQuestion>[0]) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const schema = useAnalyticsSchema();
  const fields = customLabels(t, schema.data?.entities ?? []);
  const [browsing, setBrowsing] = useState(false);
  const [entity, setEntity] = useState("");
  const saved = useSavedQuestions(entity || undefined, browsing);
  if (!browsing)
    return (
      <div>
        <Button variant="ghost" onClick={() => setBrowsing(true)}>
          {t("reporting.questionBrowse")}
        </Button>
      </div>
    );
  // An unfiltered question the report already asks is not offered again. A
  // filtered one is told apart by filters the page never sees, so it stays.
  const placed = new Set(
    layout
      .flatMap((section) => section.cards)
      .flatMap((card) =>
        card.question && !card.question.binding
          ? [sameQuestion(card.question)]
          : [],
      ),
  );
  const pickable = (saved.data?.pages ?? [])
    .flatMap((page) => page.data)
    .filter(
      (run) =>
        run.filtered ||
        !placed.has(sameQuestion(cardQuestion(savedQuestionShape(run)))),
    );
  return (
    <div className="form-stack">
      <Select
        aria-label={t("reporting.questionPopulation")}
        value={entity}
        options={[
          { value: "", label: t("reporting.questionPopulationAll") },
          ...(schema.data?.entities ?? []).map((candidate) => ({
            value: candidate.name,
            label: entityLabel(t, candidate.name),
          })),
        ]}
        onChange={setEntity}
      />
      {saved.isError && <ErrorLine error={saved.error} />}
      {saved.isPending ? (
        <p className="t-caption">{t("analytics.q.savedLoading")}</p>
      ) : (
        <Select
          aria-label={t("reporting.questionPick")}
          value=""
          placeholder={
            pickable.length > 0
              ? t("reporting.questionPickNone")
              : t("reporting.questionPickEmpty")
          }
          options={pickable.map((run) => ({
            value: run.id,
            label: `${savedQuestionLabel(t, run, fields)} · ${formatDate(run.asked_at, locale, zone)}${run.filtered ? ` · ${t("reporting.questionPickFiltered")}` : ""}`,
          }))}
          onChange={(id) => {
            const run = pickable.find((candidate) => candidate.id === id);
            if (run) onPick(run.id, savedQuestionShape(run));
          }}
        />
      )}
      {saved.hasNextPage && (
        <div>
          <Button
            variant="ghost"
            pending={saved.isFetchingNextPage}
            onClick={() => saved.fetchNextPage()}
          >
            {t("analytics.q.savedMore")}
          </Button>
        </div>
      )}
    </div>
  );
}

// A card's question as a key, so one already placed is not offered again.
function sameQuestion(question: NonNullable<Card["question"]>): string {
  return JSON.stringify([
    question.entity,
    question.group_by ?? [],
    question.measures,
    question.limit ?? null,
  ]);
}
