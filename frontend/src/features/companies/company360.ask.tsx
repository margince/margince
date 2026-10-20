import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Skeleton } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import {
  liveProjects,
  type PickableProject,
  ProjectPicker,
  useClearVanishedChoice,
  useSoleProjectDefault,
} from "../../design-system/projectpicker";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  type OpenReceipt,
  SentenceList,
  WrittenBy,
} from "../records/record360";
import "./company360.css";

type Question = components["schemas"]["CompanyQuestion"];

type Answer = components["schemas"]["CompanyAnswer"];

// The prepared questions, in the order the card offers them: what is open now,
// then what to walk in with, then what has moved.
//
// Keyed by question rather than listed, so the type is exhaustive. A question
// declared upstream with no position here fails to compile. Otherwise the
// server would answer it and the card would never ask.
const QUESTIONS: readonly Question[] = Object.keys({
  whats_open: 0,
  meeting_prep: 0,
  whats_changed: 0,
} satisfies Record<Question, 0>) as Question[];

/**
 * AskCard is "Ask Margince": three prepared questions, answered from this
 * account's own records.
 *
 * The questions are buttons, not a text box. They are indigo because the agent
 * answers them, and quiet because no one of them is the move. Each names the
 * records its answer is written from, so every sentence carries a citation the
 * reader can open. A text box answering from a subset would look the same.
 */
export function AskSection({
  companyId,
  onOpenRecord,
  onOpenReceipt,
  onOpenEmail,
  projects,
}: Readonly<{
  companyId: string;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onOpenReceipt: OpenReceipt | undefined;
  // Opens a cited message in the page's email drawer; see `Citations`.
  onOpenEmail?: (activityId: string) => void;
  // The account's projects, as the page read them. Offered as a picker
  // when any is live, so a question can be asked about one engagement
  // rather than the whole account.
  projects?: readonly PickableProject[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [projectId, setProjectId] = useState("");
  const recordZone = useRecordZone();
  const live = liveProjects(projects);
  useSoleProjectDefault(live, projectId, setProjectId);
  useClearVanishedChoice(live, projectId, setProjectId);
  const ask = useMutation({
    // The project travels as the mutation variable beside the question, so
    // a stale closure cannot ask about a project the picker does not show.
    mutationFn: async ({
      question,
      project,
    }: {
      question: Question;
      project: string;
    }) => {
      return unwrap(
        await api.POST("/companies/{id}/ask", {
          params: { path: { id: companyId } },
          body: { question, ...(project ? { project_id: project } : {}) },
        }),
      );
    },
  });

  const answer: Answer | undefined = ask.data;
  // A payload without sentences is an answer this build cannot read, not an
  // account with nothing to say. Every card here keeps the same distinction.
  const readable = Array.isArray(answer?.sentences) ? answer : undefined;
  return (
    <section className="co-part" aria-label={t("co.ask.title")}>
      <ProjectPicker
        projects={live}
        projectId={projectId}
        onChange={(next) => {
          setProjectId(next);
          // The answer on screen was written about the previous project, and
          // its scope line would otherwise stand over the next project's key.
          ask.reset();
        }}
        scope={readable?.scope}
      />
      <p className="co-ask-questions">
        {QUESTIONS.map((question) => (
          <Button
            key={question}
            variant="aiQuiet"
            onClick={() => ask.mutate({ question, project: projectId })}
            disabled={ask.isPending}
          >
            {t(`co.ask.q.${question}`)}
          </Button>
        ))}
      </p>
      {ask.isPending && <Skeleton width="100%" height={40} />}
      {ask.isError && (
        <ErrorLine>
          {t("co.ask.failed")}
          {/* The server's own detail says which failure it was. Budget exhausted
              reads differently from a malformed request, and a rep can act on one. */}
          {` ${problemMessageOf(ask.error, t)}`}
        </ErrorLine>
      )}
      {/* The previous answer is hidden while the next question is in flight.
          Left under the spinner, a finished answer sits next to a loading one.
          The reader then cannot tell which question the answer belongs to. */}
      {readable && !ask.isPending && (
        /* The answer as its own plate rather than three loose paragraphs
            under the buttons. A reply belongs in a shape that shows where it
            starts and ends. On the panel's tinted ground, the white card makes
            the prose the thing being read. */
        <div className="co-ask-answer">
          {/* The question is repeated above its answer. With three buttons and
              one answer block, a reader who scrolled cannot tell which one they
              pressed. The wrong pairing is worse than none. */}
          <p className="co-ask-asked">{t(`co.ask.q.${readable.question}`)}</p>
          {readable.sentences.length === 0 ? (
            // An empty answer is a real outcome, not a failure. This reader
            // cannot see the question's records, so there is nothing to say.
            // The card says so, rather than writing a sentence around the gap.
            <p className="surfacestate-empty">{t("co.ask.nothing")}</p>
          ) : (
            <SentenceList
              sentences={readable.sentences}
              onOpenRecord={onOpenRecord}
              onOpenReceipt={onOpenReceipt}
              onOpenEmail={onOpenEmail}
              // Gathered under the prose, not trailing each clause. An answer
              // is one reply to one question. A chip after every sentence
              // breaks the reply into a list of filed facts.
              citations="collected"
            />
          )}
          <p className="co-ask-foot co-row-meta t-caption">
            <WrittenBy by={readable.generated_by} />
            <span>
              {t("co.brief.generatedAt", {
                when: formatDate(readable.generated_at, locale, recordZone),
              })}
            </span>
          </p>
        </div>
      )}
    </section>
  );
}
