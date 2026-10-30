// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Settings } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { AiPending } from "../design-system/aipending";
import { AiReveal, useArrivalCue } from "../design-system/aireveal";
import { EmptyState } from "../design-system/atoms";
import { Button } from "../design-system/button";
import { Callout } from "../design-system/callout";
import { Eyebrow } from "../design-system/eyebrow";
import { useVisibleSettingsPages } from "../features/settings/settingsnav";
import { CitedDocument } from "../features/voice-knowledge/citeddocument";
import { AnswerView } from "../features/voice-knowledge/corpusanswer";
import {
  type Answer,
  type HandbookAnswer,
  NoDocumentSetsError,
  useHandbookAsk,
} from "../features/voice-knowledge/corpusaskqueries";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { looksLikeQuestion } from "../lib/question";
import { navigate } from "../lib/router";
import type { SettingsPageId } from "../lib/settingscatalog";
import { settingsHref } from "../lib/settingsrouting";
import type { Command } from "./palette";

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
  const { available, mutation } = useHandbookAsk(question);
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
          label: t("palette.askRow", { query: trimmed }),
          type: "action",
        }
      : null;
  // Only the answer to the words in the box: edited words are a new question.
  const shown = mutation.variables?.question === trimmed;
  const ask = () => {
    // A second Enter on the same words would pay for the answer on screen again.
    if (!(shown && (mutation.isPending || mutation.isSuccess))) {
      mutation.mutate({ question: trimmed });
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
  result: HandbookAnswer | undefined;
  // Called before a settings button leaves for its page, so the palette closes.
  onLeave: () => void;
}>) {
  const t = useT();
  const arrival = useArrivalCue(pending);
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
        <Callout tone="danger" kind="outcome" title={t("corpusAsk.failed")}>
          {problemMessageOf(error, t)}
        </Callout>
      </div>
    );
  }
  if (!result) {
    return null;
  }
  return (
    <AnsweredQuestion result={result} onLeave={onLeave} arrival={arrival} />
  );
}

function AnsweredQuestion({
  result,
  onLeave,
  arrival,
}: Readonly<{
  result: HandbookAnswer;
  onLeave: () => void;
  arrival: number | null;
}>) {
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
      <AiReveal cue={arrival}>
        <AnswerView
          answer={answer}
          openCite={openCite}
          onOpenCite={setOpenCite}
        />
      </AiReveal>
      {covered ? null : <p className="t-caption">{t("palette.askNoAnswer")}</p>}
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
      {cited ? <CitedDocument key={cited.chunk_id} claim={cited} /> : null}
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
