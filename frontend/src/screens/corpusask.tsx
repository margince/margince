import { useMutation, useQuery } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  Modal,
  Textarea,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { CitedDocument } from "./citeddocument";
import "./corpusask.css";
import { problemMessageOf, throwProblem } from "./common";

// Asking a document set a question, in the reader's own words.
//
// It is a DIALOG rather than a screen. Asking is something a reader does in the
// middle of other work — they are on a deal, a question occurs to them, and the
// answer sends them back to the deal — so it opens over the page they were on
// and gives it back when it closes. A screen made them leave and navigate back.
//
// What makes the free-text box defensible here, and what the whole surface has
// to keep visible: the search is BOUNDED. "Everything" is one finite set the
// workspace chose, so the answer can prove what it did not find — which is
// exactly what `POST /companies/{id}/ask` refused to promise, and why that
// one takes its questions from a fixed list instead.
//
// So the refusals are drawn as different things, never as one "no answer":
//
//   not_covered            the set was searched IN FULL and holds nothing close
//                          enough. The writer's own sentence says what the set
//                          DOES cover, so the reader knows where to go next
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

type Answer = components["schemas"]["KnowledgeAnswer"];
type Claim = components["schemas"]["KnowledgeClaim"];
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

export function AskMarginceModal({
  open,
  carriedQuestion,
  onClose,
}: Readonly<{
  open: boolean;
  carriedQuestion?: string;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const canAsk = useCan("knowledge_corpus", "read");
  const sets = useAskableSets(canAsk && open);
  const [corpusId, setCorpusId] = useState("");
  const [question, setQuestion] = useState("");
  // Which citation the document pane is showing, by its position in the answer.
  // Null is "nothing picked yet", which is a different pane from "picked one
  // that turned out to have no passage".
  const [openCite, setOpenCite] = useState<number | null>(null);
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

  // The palette fills the box; it does not press Ask. A question typed into a
  // palette is a question being COMPOSED — the reader was still writing it when
  // the row matched — and asking it for them spends a model call on a fragment
  // and shows them an answer to something they had not finished saying.
  const carried = carriedQuestion ?? "";
  const filled = useRef(false);
  useEffect(() => {
    if (!open) {
      filled.current = false;
      return;
    }
    if (filled.current) {
      return;
    }
    filled.current = true;
    setQuestion(carried);
  }, [open, carried]);

  const runAsk = useCallback(() => {
    setOpenCite(null);
    ask.mutate({ corpusId, question: question.trim() });
  }, [ask, corpusId, question]);

  // Only while it still belongs to the set on screen. useMutation keeps its
  // last result across a change of selection, so without this a reader who asks
  // one set, switches to another and reads on would see the FIRST set's answer
  // and citations sitting under the second set's name.
  const answer = ask.data?.corpus.id === corpusId ? ask.data : undefined;
  const claims = useMemo(() => answer?.claims ?? [], [answer]);
  const cited = openCite === null ? undefined : claims[openCite];

  return (
    // The house two-column dialog, and it is a RIGHT-SIDE drawer: `split`
    // centred has no second column to hold and falls back to the roomy box,
    // which this content overflowed. The drawer is what the design system
    // offers for an answer beside the document it came from.
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      size="split"
      placement="right"
    >
      <div className="ask-modal">
        <header className="ask-modal-head">
          <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
          <Heading size="medium" id={titleId}>
            {t("corpusAsk.title")}
          </Heading>
          {/* The set scopes the whole dialog rather than one question, which is
              why it sits in the head and not beside the box: change it and the
              NEXT ask goes somewhere else. ALWAYS drawn, even at one set — an
              answer a reader cannot attribute to a named set is an answer they
              cannot judge, and the day a second set arrives the control is
              already where they learned to look. */}
          <div className="ask-modal-set">
            {/* No visible label: the head is one line, and the control already
                says what it is by naming the set inside it. The name a screen
                reader needs rides on the control instead, so nothing is lost
                where nothing was gained by printing it twice. */}
            <Select
              aria-label={t("corpusAsk.whichSet")}
              options={(items ?? []).map((set) => ({
                value: set.id,
                label: set.name,
              }))}
              value={corpusId}
              disabled={(items?.length ?? 0) < 2}
              onChange={(next) => {
                setCorpusId(next);
                setOpenCite(null);
              }}
            />
          </div>
        </header>
        <div className="ask-modal-body">
          <section className="ask-modal-ask">
            {/* The bounded-search promise, and it stays on screen because it is
                what makes a refusal mean something: a set that answers
                everything is worth less than one that says when it cannot. */}
            <p className="t-sub">{t("corpusAsk.sub")}</p>
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
                // The one AI call to action on this surface: the model does the
                // reading and writes the sentence.
                variant="ai"
                disabled={question.trim() === "" || corpusId === ""}
                pending={ask.isPending}
                onClick={runAsk}
              >
                {t("corpusAsk.submit")}
              </Button>
            </div>
            {!canAsk || (items && items.length === 0) ? (
              <EmptyState title={t("corpusAsk.noSetsTitle")}>
                <p>{t("corpusAsk.noSets")}</p>
              </EmptyState>
            ) : null}
            {ask.isError ? (
              <Callout
                tone="danger"
                kind="outcome"
                title={t("corpusAsk.failed")}
              >
                {problemMessageOf(ask.error, t)}
              </Callout>
            ) : null}
            {answer ? (
              <AnswerView
                answer={answer}
                openCite={openCite}
                onOpenCite={setOpenCite}
              />
            ) : null}
          </section>
          <section className="ask-modal-doc">
            {/* Keyed on the citation: the pane's own "could this be marked"
                verdict belongs to ONE passage, and React keeps state across a
                prop change unless told the subject changed. */}
            <CitedDocument key={cited?.chunk_id ?? "none"} claim={cited} />
          </section>
        </div>
      </div>
    </Modal>
  );
}

// CITE_MARKER is the bracketed number the writer puts at the end of each
// sentence of its summary — [1] for its first claim, counting in the order it
// listed them. It is what turns a paragraph into something a reader can check:
// a sentence with no number beside it is one they cannot follow to a document.
const CITE_MARKER = /\[(\d+)\]/g;

function AnswerView({
  answer,
  openCite,
  onOpenCite,
}: Readonly<{
  answer: Answer;
  openCite: number | null;
  onOpenCite: (at: number | null) => void;
}>) {
  const t = useT();
  if (answer.outcome !== "answered" && answer.outcome !== "unreviewed") {
    return <Refusal answer={answer} />;
  }
  const claims = answer.claims ?? [];
  return (
    <div className="form-stack">
      {/* NOBODY READ THESE. The passages are what the search ranked nearest,
          and under `unreviewed` no writer judged whether they answer the
          question — ranking alone cannot, so a reader who is not told would
          read the nearest passage as the answer. */}
      {answer.outcome === "unreviewed" ? (
        <Callout
          tone="warning"
          kind="outcome"
          title={t("corpusAsk.unreviewedTitle")}
        >
          {t("corpusAsk.unreviewed")}
        </Callout>
      ) : null}
      {/* WHO WROTE THIS. Never omitted, and never inferred from whether the
          claims carry sentences: a reader deciding how much to trust a line
          needs to be told, not to work it out from the shape of the page. The
          head's badge says this SURFACE is AI-assisted, which is a different
          claim — a deterministic answer arrives on the same surface and nobody
          wrote a word of it. */}
      <Badge tone={answer.generated_by === "model" ? "ai" : undefined}>
        {answer.generated_by === "model"
          ? t("corpusAsk.byModel")
          : t("corpusAsk.byPassages")}
      </Badge>
      {answer.summary ? (
        <p className="ask-summary">
          <Summary
            summary={answer.summary}
            claims={claims}
            openCite={openCite}
            onOpenCite={onOpenCite}
          />
        </p>
      ) : null}
      {/* The claims, when no summary carried them. A deterministic answer never
          has one — nothing wrote prose — and the quotes then stand on their
          own, which is honest: the grounded part was never the sentence. */}
      {answer.summary ? null : (
        <ul className="ask-bare-claims">
          {claims.map((claim, at) => (
            <li key={claim.chunk_id}>
              {claim.text ? <span>{claim.text} </span> : null}
              <CiteButton
                at={at}
                claim={claim}
                active={openCite === at}
                onOpenCite={onOpenCite}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// Summary renders the writer's paragraph with its markers turned into the
// buttons that open each passage.
//
// A marker naming a claim the answer does not carry is DROPPED rather than
// drawn dead: the number came from a model, and a button that opens nothing is
// worse than a sentence that is merely uncited.
function Summary({
  summary,
  claims,
  openCite,
  onOpenCite,
}: Readonly<{
  summary: string;
  claims: readonly Claim[];
  openCite: number | null;
  onOpenCite: (at: number | null) => void;
}>) {
  const parts: React.ReactNode[] = [];
  let cut = 0;
  for (const found of summary.matchAll(CITE_MARKER)) {
    const at = Number(found[1]) - 1;
    const claim = claims[at];
    if (found.index === undefined) {
      continue;
    }
    parts.push(summary.slice(cut, found.index));
    cut = found.index + found[0].length;
    if (claim) {
      parts.push(
        <CiteButton
          key={`${claim.chunk_id}-${at}`}
          at={at}
          claim={claim}
          active={openCite === at}
          onOpenCite={onOpenCite}
        />,
      );
    }
  }
  parts.push(summary.slice(cut));
  return <>{parts}</>;
}

function CiteButton({
  at,
  claim,
  active,
  onOpenCite,
}: Readonly<{
  at: number;
  claim: Claim;
  active: boolean;
  onOpenCite: (at: number | null) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  // The document and the line ride in the accessible name rather than only in a
  // tooltip: the number alone says nothing about where it goes, and a reader on
  // a screen reader gets no hover.
  const where = claim.line
    ? t("corpusAsk.citeAtLine", {
        document: claim.document_name,
        line: formatNumber(claim.line, locale),
      })
    : t("corpusAsk.citeInDocument", { document: claim.document_name });
  return (
    <button
      type="button"
      className={active ? "ask-cite ask-cite-on" : "ask-cite"}
      aria-label={where}
      aria-pressed={active}
      title={where}
      onClick={() => onOpenCite(active ? null : at)}
    >
      {formatNumber(at + 1, locale)}
    </button>
  );
}

function Refusal({ answer }: Readonly<{ answer: Answer }>) {
  const t = useT();
  // Counts, so they are MAGNITUDES and take the reader's own notation: a German
  // reader seeing 1234 beside a formatted 1.234 is one screen written in two.
  const { locale } = useLocale();
  if (answer.outcome === "not_ready") {
    return (
      // The same plate the not_covered branch below draws: all three refusals
      // stand where the answer would have been, so a reader who pressed Ask
      // and got none reads one shape rather than three.
      <EmptyState title={t("corpusAsk.notReadyTitle")}>
        <p>
          {t("corpusAsk.notReady", {
            embedded: formatNumber(answer.coverage.chunks_embedded, locale),
            total: formatNumber(answer.coverage.chunks_total, locale),
          })}
        </p>
      </EmptyState>
    );
  }
  if (answer.outcome === "retrieval_unavailable") {
    return (
      <EmptyState title={t("corpusAsk.retrievalUnavailableTitle")}>
        <p>{t("corpusAsk.retrievalUnavailable")}</p>
      </EmptyState>
    );
  }
  // not_covered. The WRITER'S own sentence leads, because it names what this
  // set does cover instead — the topic statement is the fallback for an answer
  // that arrived without one, never a second paragraph beside it.
  return (
    <EmptyState title={t("corpusAsk.notCovered.title")}>
      <p>
        {answer.summary ??
          t("corpusAsk.notCovered.body", { name: answer.corpus.name })}
      </p>
      {answer.summary ? null : (
        <blockquote>{answer.corpus.topic_statement}</blockquote>
      )}
    </EmptyState>
  );
}
