// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { Settings } from "lucide-react";
import { Suspense, useEffect, useMemo, useState } from "react";
import { AiPending } from "../design-system/aipending";
import { AiReveal } from "../design-system/aireveal";
import { EmptyState } from "../design-system/atoms";
import { Button } from "../design-system/button";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { useVisibleSettingsPages } from "../features/settings/settingsnav";
import {
  type Answer,
  NoDocumentSetsError,
  useHandbookAsk,
} from "../features/voice-knowledge/corpusaskqueries";
import { useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { grants } from "../lib/accessgrants";
import { problemMessageOf } from "../lib/problem";
import { looksLikeDataQuestion, looksLikeQuestion } from "../lib/question";
import { navigate } from "../lib/router";
import { useMe } from "../lib/session";
import type { SettingsPageId } from "../lib/settingscatalog";
import { settingsHref } from "../lib/settingsrouting";
import type { Command } from "./palette";

import {
  askPaletteQuestion,
  type HandbookFallback,
  type PaletteAskAnswer,
} from "./palettedata.queries";

const PaletteDataAnswer = lazy(async () => ({
  default: (await import("./palettedata")).PaletteDataAnswer,
}));

const CitedDocument = lazy(async () => ({
  default: (await import("../features/voice-knowledge/citeddocument"))
    .CitedDocument,
}));
const AnswerView = lazy(async () => ({
  default: (await import("../features/voice-knowledge/corpusanswer"))
    .AnswerView,
}));

type Claim = NonNullable<Answer["claims"]>[number];

/**
 * The search box's ask. A question no destination matched gets a row that
 * leads the list, so Enter asks it. The hook also holds the answer it got.
 */
export function usePaletteAsk(
  query: string,
  open: boolean,
  destinations: number,
) {
  const t = useT();
  const trimmed = query.trim();
  const question = looksLikeQuestion(trimmed);
  const handbook = useHandbookAsk(question);
  const me = useMe(question);
  const readData =
    grants(me.data, "installation_settings", "read") &&
    (grants(me.data, "deal", "read") ||
      grants(me.data, "contact", "read") ||
      grants(me.data, "company", "read") ||
      grants(me.data, "lead", "read") ||
      grants(me.data, "project", "read") ||
      grants(me.data, "report_definition", "read"));
  const available =
    handbook.available || (readData && looksLikeDataQuestion(trimmed));
  const mutation = useMutation({ mutationFn: askPaletteQuestion });
  const { reset } = mutation;
  useEffect(() => {
    if (open) {
      reset();
    }
  }, [open, reset]);
  const row: Command | null =
    available && destinations === 0 && question
      ? {
          id: "ask-ai",
          label: t(
            readData && looksLikeDataQuestion(trimmed)
              ? "palette.askDataRow"
              : "palette.askRow",
            { query: trimmed },
          ),
          type: "action",
        }
      : null;
  // Only the answer to the words in the box: edited words are a new question.
  const shown = mutation.variables?.question === trimmed;
  const ask = () => {
    // A second Enter on the same words would pay for the answer on screen again.
    if (!(shown && (mutation.isPending || mutation.isSuccess))) {
      mutation.mutate({
        question: trimmed,
        readData,
        askHandbook: handbook.mutation.mutateAsync,
        handbookAvailable: handbook.available,
      });
    }
  };
  return { row, shown, ask, mutation };
}

/**
 * The answer to a question asked in the search box, drawn above the record
 * rows so both show at once. A question still being asked shows a pending
 * line; a failed ask says so and leaves the rows below working.
 */
export function PaletteAnswer({
  pending,
  error,
  result,
  onLeave,
}: Readonly<{
  pending: boolean;
  error: unknown;
  result: PaletteAskAnswer | undefined;
  // Called before a settings button leaves for its page, so the palette closes.
  onLeave: () => void;
}>) {
  const t = useT();
  if (pending) {
    return (
      <div className="palette-answer">
        <AiPending label={t("palette.asking")} lines={2} />
      </div>
    );
  }
  if (error instanceof NoDocumentSetsError) {
    return (
      <div className="palette-answer">
        <EmptyState title={t("corpusAsk.noSetsTitle")}>
          <p>{t("corpusAsk.noSets")}</p>
        </EmptyState>
      </div>
    );
  }
  if (error) {
    return (
      <div className="palette-answer">
        <DialogRefusal title={t("corpusAsk.failed")} error={error} />
      </div>
    );
  }
  if (!result) {
    return null;
  }
  if ("dataAnswer" in result)
    return (
      <Suspense fallback={<AiPending label={t("palette.asking")} lines={2} />}>
        <PaletteDataAnswer answer={result.dataAnswer} onLeave={onLeave} />
      </Suspense>
    );
  if ("unsupportedData" in result)
    return (
      <section className="palette-answer" aria-label={t("palette.answerAria")}>
        <p>{t("palette.dataUnsupportedOnly")}</p>
        <DialogRefusal title={t("palette.dataUnavailable")} error={result.dataError} />
        <Button
          onClick={() => {
            onLeave();
            navigate({ screen: "analytics" });
          }}
        >
          {t("palette.openAnalytics")}
        </Button>
      </section>
    );
  return <AnsweredQuestion result={result} onLeave={onLeave} />;
}

function AnsweredQuestion({
  result,
  onLeave,
}: Readonly<{ result: HandbookFallback; onLeave: () => void }>) {
  const t = useT();
  const { answer, asked } = result;
  const [openCite, setOpenCite] = useState<number | null>(null);
  const claims = answer.claims ?? [];
  const cited = openCite === null ? undefined : claims[openCite];
  const settings = useOpenableSettings(claims);
  const sections = distinct(claims.map((claim) => claim.section?.title));
  const covered = answer.outcome !== "not_covered";
  return (
    <section className="palette-answer" aria-label={t("palette.answerAria")}>
      <Eyebrow>
        {covered && asked.length > 1
          ? t("palette.answerAfterHandbook", { name: answer.corpus.name })
          : t("palette.answerFrom", { name: answer.corpus.name })}
      </Eyebrow>
      <AiReveal text={answer.summary}>
        <Suspense fallback={null}>
          <AnswerView
            answer={answer}
            openCite={openCite}
            onOpenCite={setOpenCite}
          />
        </Suspense>
      </AiReveal>
      {result.dataError ? (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("palette.dataUnavailable")}
        >
          {problemMessageOf(result.dataError, t)}
        </Callout>
      ) : null}
      {result.dataAttempted ? <p>{t("palette.dataUnsupported")}</p> : null}
      {covered ? null : looksLikeDataQuestion(result.question) ? (
        <Button
          onClick={() => {
            onLeave();
            navigate({ screen: "analytics" });
          }}
        >
          {t("palette.openAnalytics")}
        </Button>
      ) : (
        <p className="t-caption">{t("palette.askNoAnswer")}</p>
      )}
      {sections.length > 0 ? (
        <ul className="palette-answer-sections t-caption">
          {sections.map((title) => (
            <li key={title}>{t("palette.answerSection", { title })}</li>
          ))}
        </ul>
      ) : null}
      {settings.length > 0 ? (
        <div className="form-actions">
          {settings.map((page) => (
            <Button
              key={page}
              onClick={() => {
                onLeave();
                navigate(settingsHref(page));
              }}
            >
              <Settings aria-hidden />
              {t("palette.openSetting", {
                page: t(`settings.tab.${page}`),
              })}
            </Button>
          ))}
        </div>
      ) : null}
      {cited ? (
        <Suspense fallback={null}>
          <CitedDocument key={cited.chunk_id} claim={cited} />
        </Suspense>
      ) : null}
    </section>
  );
}

// The settings pages the answer's sections name, kept only where this reader
// may open the page: a button to a refusal is worse than none.
function useOpenableSettings(
  claims: readonly Claim[],
): readonly SettingsPageId[] {
  const visible = useVisibleSettingsPages();
  return useMemo(() => {
    const named = distinct(claims.map((claim) => claim.opens?.settings_page));
    return visible.map((page) => page.id).filter((id) => named.includes(id));
  }, [claims, visible]);
}

function distinct(values: readonly (string | undefined)[]): string[] {
  return [
    ...new Set(values.filter((value): value is string => value !== undefined)),
  ];
}
