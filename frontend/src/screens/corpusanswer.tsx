// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { InlineMarkdown } from "../design-system/markdown";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";

// What an ANSWER looks like once there is one, including every way there is not
// one. Its own file because the dialog around it is a different concern: that
// one owns the question, the set and which citation is open, and this one owns
// what a reader is shown for a reply that arrived.
//
// The refusals are drawn as different things, never as one "no answer":
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

// CITE_MARKER is the bracketed number the writer puts at the end of each
// sentence of its summary — [1] for its first claim, counting in the order it
// listed them. It is what turns a paragraph into something a reader can check:
// a sentence with no number beside it is one they cannot follow to a document.
const CITE_MARKER = /\[(\d+)\]/g;

/** Whether any marker in `summary` names a claim the answer actually carries. */
function resolvesAnyCite(summary: string, claims: readonly Claim[]): boolean {
  for (const found of summary.matchAll(CITE_MARKER)) {
    if (claims[Number(found[1]) - 1]) return true;
  }
  return false;
}

export function AnswerView({
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
  // A summary leads ONLY when its markers reach the claims beside it. The
  // numbers come from a model; one that wrote none, or wrote only numbers no
  // claim answers to, has written a paragraph a reader cannot check.
  const summary = answer.summary ?? "";
  const showSummary = summary !== "" && resolvesAnyCite(summary, claims);
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
      {showSummary ? (
        <p className="ask-summary">
          <Summary
            summary={summary}
            claims={claims}
            openCite={openCite}
            onOpenCite={onOpenCite}
          />
        </p>
      ) : null}
      {/* The claims, when the summary did not carry them. A deterministic answer
          has no summary at all — nothing wrote prose — and a written one whose
          markers resolve to nothing is the same situation for a reader: a
          paragraph with its evidence off screen, which is the shape the coverage
          field was added to prevent. The quotes then stand on their own. */}
      {showSummary ? null : (
        <ul className="ask-bare-claims">
          {claims.map((claim, at) => (
            // Position is identity: one immutable answer's claims, in order,
            // and two of them can be quoted from the SAME chunk.
            // biome-ignore lint/suspicious/noArrayIndexKey: positional by nature
            <li key={at}>
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
    parts.push(
      <InlineMarkdown
        key={`t${cut}`}
        links={false}
        text={summary.slice(cut, found.index)}
      />,
    );
    cut = found.index + found[0].length;
    if (claim) {
      parts.push(
        <CiteButton
          key={`c${cut}`}
          at={at}
          claim={claim}
          active={openCite === at}
          onOpenCite={onOpenCite}
        />,
      );
    }
  }
  parts.push(
    <InlineMarkdown key={`t${cut}`} links={false} text={summary.slice(cut)} />,
  );
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
  const number = formatNumber(at + 1, locale);
  // The document and the line ride in the accessible name rather than only in a
  // tooltip: the number alone says nothing about where it goes, and a reader on
  // a screen reader gets no hover.
  //
  // The VISIBLE number opens that name, which is WCAG 2.5.3: a voice-control
  // user says "click 1" and the control has to answer to what it shows. A name
  // that began at the document left the only label they can see out of it.
  const where = claim.line
    ? t("corpusAsk.citeAtLine", {
        number,
        document: claim.document_name,
        line: formatNumber(claim.line, locale),
      })
    : t("corpusAsk.citeInDocument", { number, document: claim.document_name });
  return (
    <button
      type="button"
      className={active ? "ask-cite ask-cite-on" : "ask-cite"}
      aria-label={where}
      aria-pressed={active}
      title={where}
      onClick={() => onOpenCite(active ? null : at)}
    >
      {number}
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
