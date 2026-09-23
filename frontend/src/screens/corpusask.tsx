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
import { useT } from "../i18n";
import { CitedDocument } from "./citeddocument";
import { AnswerView } from "./corpusanswer";
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
  //
  // Keyed on the QUESTION rather than on having filled once: the dialog is not
  // remounted between one carried question and the next — the address changes
  // under it — so a boolean guard left the second reader looking at the first
  // reader's question with their own nowhere on screen.
  const carried = carriedQuestion ?? "";
  const filled = useRef<string | null>(null);
  useEffect(() => {
    if (!open) {
      filled.current = null;
      return;
    }
    if (filled.current === carried) {
      return;
    }
    filled.current = carried;
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
        {/* The question spans the dialog, because asking is what the reader
            came to do and the box is not half of anything. */}
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
          {/* Two different facts, and the reader can act on only one of them.
              A company with no documents filed has nothing to search; a reader
              without the grant is looking at a company that may be full of
              them. Telling the second they have no documents is a false
              statement about somebody else's data. */}
          {canAsk ? null : (
            <EmptyState title={t("corpusAsk.noGrantTitle")}>
              <p>{t("corpusAsk.noGrant")}</p>
            </EmptyState>
          )}
          {canAsk && items && items.length === 0 ? (
            <EmptyState title={t("corpusAsk.noSetsTitle")}>
              <p>{t("corpusAsk.noSets")}</p>
            </EmptyState>
          ) : null}
          {ask.isError ? (
            <Callout tone="danger" kind="outcome" title={t("corpusAsk.failed")}>
              {problemMessageOf(ask.error, t)}
            </Callout>
          ) : null}
        </section>
        {/* The document arrives only when a reader asks for it, and the answer
            has the width to itself until then. A pane held open on an empty
            state spends half the dialog saying nothing — and on a refusal it
            says nothing FOREVER, because a refusal has no citation to press. */}
        <div className="ask-modal-body">
          <section className="ask-modal-answer">
            {answer ? (
              <AnswerView
                answer={answer}
                openCite={openCite}
                onOpenCite={setOpenCite}
              />
            ) : null}
          </section>
          {cited ? (
            <section className="ask-modal-doc">
              <CitedDocument key={cited.chunk_id} claim={cited} />
            </section>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}
