import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../api/schema";
import { EmptyState } from "../design-system/atoms";
import {
  Markdown,
  type MarkdownHighlightOutcome,
} from "../design-system/markdown";
import { useT } from "../i18n";

// The document half of the ask dialog: the file a citation points at, with the
// quoted passage marked in it.
//
// A citation nobody can follow is a citation in name only. The reader has the
// sentence and the quote; this is what lets them see the quote where it was
// written, with what surrounds it — which is the part that tells them whether
// the sentence is a fair reading of it.

type Claim = components["schemas"]["KnowledgeClaim"];

// The document's own bytes, as the answer's citation points at them. Fetched
// through the browser rather than the generated client because the endpoint
// serves a FILE — the client decodes JSON, and a markdown page is not.
function useDocumentText(documentId: string | undefined) {
  return useQuery({
    enabled: documentId !== undefined,
    queryKey: ["knowledge-document-text", documentId],
    queryFn: async () => {
      const answer = await fetch(`/v1/knowledge/documents/${documentId}`, {
        credentials: "same-origin",
      });
      if (!answer.ok) {
        throw new Error(String(answer.status));
      }
      return answer.text();
    },
  });
}

export function CitedDocument({ claim }: Readonly<{ claim: Claim }>) {
  const t = useT();
  const document = useDocumentText(claim.document_id);
  // What the renderer managed to mark. A miss on one passage says nothing about
  // the next, so this starts over per citation — which the CALLER arranges with
  // a key rather than an effect resetting it after a render that already drew
  // the previous answer's verdict over the new passage.
  const [found, setFound] = useState<MarkdownHighlightOutcome>("quote");

  if (document.isPending) {
    return <p className="t-caption">{t("corpusAsk.documentLoading")}</p>;
  }
  // A document that will not open is said so plainly, with the quote kept on
  // screen: the reader came here to read that span, and the quote is the part
  // of it we already have.
  if (document.isError || document.data === undefined) {
    return (
      <EmptyState title={t("corpusAsk.documentFailedTitle")}>
        <p>
          {t("corpusAsk.documentFailed", { document: claim.document_name })}
        </p>
        <blockquote>{claim.quote}</blockquote>
      </EmptyState>
    );
  }
  return (
    <div className="ask-doc">
      <header className="ask-doc-head">
        <span className="ask-doc-name">{claim.document_name}</span>
        <a
          className="t-caption"
          href={`/v1/knowledge/documents/${claim.document_id}`}
        >
          {t("corpusAsk.openFile")}
        </a>
      </header>
      {/* Roughly one quote in four cannot be pinpointed — it crosses a line
          break in the source, or survived the server's check only once
          whitespace was collapsed. The reader is TOLD, rather than left to
          wonder why nothing is marked on a page they were sent to. */}
      {found === "none" ? (
        <p className="ask-doc-note t-caption">
          {t("corpusAsk.quoteNotPinpointed")}
        </p>
      ) : null}
      <div className="ask-doc-body">
        <Markdown
          source={document.data}
          highlight={{ quote: claim.quote, line: claim.line }}
          onHighlight={setFound}
        />
      </div>
    </div>
  );
}
