import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Textarea,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { FileChip } from "../design-system/filechip";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";

// Asking a document set a question, in the reader's own words.
//
// What makes the free-text box defensible here, and what the whole surface has
// to keep visible: the search is BOUNDED. "Everything" is one finite set the
// workspace chose, so the answer can prove what it did not find — which is
// exactly what `POST /organizations/{id}/ask` refused to promise, and why that
// one takes its questions from a fixed list instead.
//
// So the three refusals are drawn as three different things, never as one
// "no answer" state:
//
//   not_covered            the set was searched IN FULL and holds nothing close
//                          enough — and the set's own topic statement is quoted
//                          back, so the reader learns what it is FOR
//   not_ready              the set is mid-ingest or being re-read. Nothing is
//                          wrong with the question
//   retrieval_unavailable  no search lane is configured, so nothing was
//                          searched at all. Nothing is wrong with the question
//                          OR the set
//
// `unreviewed` is not a refusal and is not an answer. The set WAS searched and
// the nearest passages are on screen, but no writer read them — so the reader
// is told that in a callout above them, because a passage presented like an
// answer is read as one.
//
// And `generated_by` is on screen whenever there is an answer. A reader
// deciding how much to trust a sentence needs to know whether a model wrote it
// or whether they are looking at the passages themselves.

type Answer = components["schemas"]["KnowledgeAnswer"];
type Corpus = components["schemas"]["KnowledgeCorpus"];

function useAskableSets(enabled: boolean) {
  return useQuery({
    enabled,
    queryKey: ["knowledge-corpora"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/knowledge/corpora");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useAsk() {
  return useMutation({
    mutationFn: async ({
      corpusId,
      question,
    }: {
      corpusId: string;
      question: string;
    }): Promise<Answer> => {
      const { data, error } = await api.POST("/knowledge/corpora/{id}/ask", {
        params: { path: { id: corpusId } },
        body: { question },
      });
      if (error || !data) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// preferredSet is the set the box starts on: the one the workspace marked as
// the palette's target, else the first. Never an empty selection — a reader who
// arrived carrying a question should not have to choose a set before they can
// ask it.
function preferredSet(sets: readonly Corpus[]): string {
  const marked = sets.find((set) => set.default_ask);
  return marked?.id ?? sets[0]?.id ?? "";
}

export function CorpusAskCard({
  carriedQuestion,
}: Readonly<{ carriedQuestion?: string | null }>) {
  const t = useT();
  const canAsk = useCan("knowledge_corpus", "read");
  const sets = useAskableSets(canAsk);
  const [corpusId, setCorpusId] = useState("");
  const [question, setQuestion] = useState(carriedQuestion ?? "");
  const ask = useAsk();

  // The set is chosen once the list arrives, and only while nothing is chosen:
  // re-running it on every render would take the reader's own choice away the
  // moment the list refetched.
  const items = sets.data?.items;
  useEffect(() => {
    if (items && corpusId === "") {
      setCorpusId(preferredSet(items));
    }
  }, [items, corpusId]);

  // Nothing is offered until we KNOW there is something to ask. The three
  // cases collapse to one answer — no grant, no sets, or not yet told — and
  // that is deliberate: rendering the box while the list is still in flight
  // shows a reader an input that then vanishes under them, which is worse than
  // a card that arrives a moment late. A box that answers nothing is worse
  // than no box.
  if (!canAsk || items === undefined || items.length === 0) {
    return null;
  }

  return (
    <Panel title={t("corpusAsk.title")}>
      <PanelBody className="form-stack">
        <p className="t-caption">{t("corpusAsk.sub")}</p>
        {items && items.length > 1 ? (
          <Field label={t("corpusAsk.whichSet")}>
            {(control) => (
              <Select
                {...control}
                options={items.map((set) => ({
                  value: set.id,
                  label: set.name,
                }))}
                value={corpusId}
                onChange={setCorpusId}
              />
            )}
          </Field>
        ) : null}
        <Field label={t("corpusAsk.question")}>
          {(control) => (
            <Textarea
              {...control}
              value={question}
              onChange={(event) => setQuestion(event.target.value)}
            />
          )}
        </Field>
        <div className="form-actions">
          <Button
            disabled={question.trim() === "" || corpusId === ""}
            pending={ask.isPending}
            onClick={() => ask.mutate({ corpusId, question: question.trim() })}
          >
            {t("corpusAsk.submit")}
          </Button>
        </div>
        {ask.isError ? (
          <Callout tone="danger">{problemMessageOf(ask.error, t)}</Callout>
        ) : null}
        {/* Only while it still belongs to the set on screen. useMutation keeps
            its last result across a change of selection, so without this a
            reader who asks one set, switches to another, and reads on would
            see the FIRST set's answer and citations sitting under the second
            set's name — with nothing on the page saying so. */}
        {ask.data && ask.data.corpus.id === corpusId ? (
          <AnswerView answer={ask.data} />
        ) : null}
      </PanelBody>
    </Panel>
  );
}

function AnswerView({ answer }: Readonly<{ answer: Answer }>) {
  const t = useT();
  // A line and a column are MAGNITUDES a person counts with, so they take the
  // reader's own notation like every other figure on the page.
  const { locale } = useLocale();
  if (answer.outcome !== "answered" && answer.outcome !== "unreviewed") {
    return <Refusal answer={answer} />;
  }
  const claims = answer.claims ?? [];
  return (
    <div className="form-stack">
      {/* NOBODY READ THESE. The passages are what the search ranked nearest,
          and under `unreviewed` no writer judged whether they answer the
          question — ranking alone cannot, so a reader who is not told would
          read the nearest passage as the answer. It leads the panel rather
          than sitting under it for that reason. */}
      {answer.outcome === "unreviewed" ? (
        <Callout tone="warn">{t("corpusAsk.unreviewed")}</Callout>
      ) : null}
      {/* WHO WROTE THIS. Never omitted, and never inferred from whether the
          claims carry sentences: a reader deciding how much to trust a line
          needs to be told, not to work it out from the shape of the page. */}
      <Badge tone={answer.generated_by === "model" ? "ai" : undefined}>
        {answer.generated_by === "model"
          ? t("corpusAsk.byModel")
          : t("corpusAsk.byPassages")}
      </Badge>
      {claims.map((claim) => (
        <PanelRow key={claim.chunk_id}>
          <div className="form-stack">
            {/* Absent when nobody wrote a sentence, which is the deterministic
                answer's shape. The quote then stands on its own, which is
                honest: the grounded part of a grounded answer was never the
                prose. */}
            {claim.text ? <p>{claim.text}</p> : null}
            <blockquote className="t-caption">{claim.quote}</blockquote>
            {/* The file itself, downloadable, beside where in it the quote
                sits. A citation nobody can follow is a citation in name only —
                the reader has the sentence and the quote, and this is what lets
                them open the document and see it in place. */}
            <div className="card-actions">
              <FileChip
                href={`/v1/knowledge/documents/${claim.document_id}`}
                filename={claim.document_name}
              />
              {claim.line ? (
                <span className="t-caption">
                  {t("corpusAsk.atLine", {
                    line: formatNumber(claim.line, locale),
                    column: formatNumber(claim.column ?? 1, locale),
                  })}
                </span>
              ) : null}
            </div>
          </div>
        </PanelRow>
      ))}
    </div>
  );
}

function Refusal({ answer }: Readonly<{ answer: Answer }>) {
  const t = useT();
  // Counts, so they are MAGNITUDES and take the reader's own notation: a German
  // reader seeing 1234 beside a formatted 1.234 is one screen written in two.
  const { locale } = useLocale();
  if (answer.outcome === "not_ready") {
    return (
      <Callout tone="info">
        {t("corpusAsk.notReady", {
          embedded: formatNumber(answer.coverage.chunks_embedded, locale),
          total: formatNumber(answer.coverage.chunks_total, locale),
        })}
      </Callout>
    );
  }
  if (answer.outcome === "retrieval_unavailable") {
    return <Callout tone="warn">{t("corpusAsk.retrievalUnavailable")}</Callout>;
  }
  // not_covered: the set WAS searched, in full. The topic statement is quoted
  // back because it is the only thing on screen that tells the reader what this
  // set is for — and they are reading it at their least patient moment.
  return (
    <EmptyState title={t("corpusAsk.notCovered.title")}>
      <p>{t("corpusAsk.notCovered.body", { name: answer.corpus.name })}</p>
      <blockquote>{answer.corpus.topic_statement}</blockquote>
    </EmptyState>
  );
}
