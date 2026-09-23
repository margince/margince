// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, type ReactNode, useEffect, useMemo, useRef } from "react";
import { TableScroll } from "./atoms";
import "./atoms.css";
import { runText } from "./markdown-highlight";
import {
  type Block,
  type Inline,
  type MarkdownHighlight,
  type MarkdownHighlightOutcome,
  parseInline,
  type Run,
  readMarkdown,
} from "./markdown-parse";
import "./markdown.css";

export type { MarkdownHighlight, MarkdownHighlightOutcome };

/**
 * Markdown — a knowledge-corpus document, read-only, with one passage marked.
 *
 * It opens beside an answer so a reader can see the sentence a citation quotes
 * in the paragraph it came from. That is its whole job: no editing, no anchors
 * to hand out, no table of contents.
 *
 * ## Why this is hand-rolled
 *
 * The same reason `RichText` has no editor library: a dependency is carried
 * forever and this frontend runs on six of them. A markdown library would
 * bring a parser, an AST, a plugin system and — in every popular one — an HTML
 * passthrough that is switched off by a flag. The flag is the problem. This
 * corpus holds documents a customer uploaded, so the renderer has to be one
 * where "render the author's HTML" is not a setting that exists.
 *
 * ## Everything on the page is an element this file named
 *
 * There is no `dangerouslySetInnerHTML` here and no string that becomes markup.
 * `markdown-parse.ts` returns data; this file turns each shape into an element
 * from a closed list and puts the source's own characters in as TEXT, which
 * React escapes. A `<script>` tag in the corpus is therefore visible on the
 * page as eight characters and is never a script — which is also what a reader
 * auditing an uploaded document needs to see.
 *
 * A link's href passes the allowlist in `safeHref` or it is not a link at all.
 *
 * ## The highlight, and the third outcome
 *
 * `highlight.quote` is matched with whitespace collapsed on both sides, the
 * same normalisation the server locates a claim under
 * (`compose/corpusaskreply.go`). About one citation in four still misses —
 * the quote crosses a line break, or the model re-wrapped it — so
 * `highlight.line` is the second try and marks the block that line falls in.
 *
 * When neither lands the document renders plain and `onHighlight` is called
 * with `"none"`. A caller that wants to say "we could not find this passage"
 * has to be told, and a viewer that silently showed an unmarked document would
 * read as one where the citation was simply wrong.
 */
/**
 * One line of markdown, rendered inline: bold, italic, code and links, with no
 * block of its own.
 *
 * It exists because a written ANSWER is markdown too. The model that quotes a
 * handbook writes in the handbook's idiom, so its sentence arrives carrying
 * "**Settings → Data model → Pipelines**" — which, printed as text, shows a
 * reader the asterisks and tells them the product cannot read its own writer.
 * Same parser and same renderer as the document beside it, so the two cannot
 * come to disagree about what a star means, and same closed element list, so a
 * sentence is no more able to inject markup than a document is.
 */
export function InlineMarkdown({ text }: Readonly<{ text: string }>) {
  return (
    <>
      {parseInline(text).map((node, at) => (
        // The index IS the identity: these are the nodes of one immutable
        // sentence, in order, and nothing reorders or removes one.
        // biome-ignore lint/suspicious/noArrayIndexKey: positional by nature
        <Fragment key={at}>{renderInline(node)}</Fragment>
      ))}
    </>
  );
}

export function Markdown({
  source,
  highlight,
  onHighlight,
}: Readonly<{
  /** The raw document. Every byte of it is treated as hostile. */
  source: string;
  highlight?: MarkdownHighlight;
  /**
   * Which of the three outcomes the highlight reached, reported on every
   * change of `source` or `highlight` — `"none"` included, which is the one a
   * caller has to render a sentence for.
   */
  onHighlight?: (outcome: MarkdownHighlightOutcome) => void;
}>) {
  const quote = highlight?.quote;
  const line = highlight?.line;
  // Keyed on the two values rather than on the object: a caller building
  // `{ quote, line }` in its own render would otherwise re-parse the document
  // on every keystroke anywhere above it.
  const doc = useMemo(
    () =>
      readMarkdown(source, quote === undefined ? undefined : { quote, line }),
    [source, quote, line],
  );
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    onHighlight?.(doc.outcome);
  }, [doc, onHighlight]);

  useEffect(() => {
    // Nothing was marked, so there is nothing to bring into view and the
    // reader keeps the top of the document they were given.
    if (doc.outcome === "none") return;
    // The first mark in reading order, whichever of the two outcomes put it
    // there. `block: "center"` rather than the top: a passage pinned to the
    // ceiling loses the sentence before it, which is the context that makes a
    // quote readable.
    const target = root.current?.querySelector("mark, [data-markdown-mark]");
    if (target instanceof HTMLElement) {
      target.scrollIntoView({ block: "center" });
    }
  }, [doc]);

  return (
    <div className="md" ref={root}>
      {renderBlocks(doc.blocks)}
    </div>
  );
}

/**
 * Render a parsed sequence, keyed by position.
 *
 * Position IS identity in a document: the whole tree is re-derived from
 * `source` on every change, so nothing ever moves relative to its neighbours
 * without the list around it being rebuilt, and there is no id to key on
 * instead. Said once, here, rather than waived at every map that would
 * otherwise need it.
 *
 * The `Fragment` carries the key so a `<li>` or a `<tr>` stays the direct child
 * its parent element requires — a fragment draws nothing of its own.
 */
function sequence<T>(
  items: readonly T[],
  render: (item: T) => ReactNode,
): ReactNode[] {
  return items.map((item, index) => (
    // biome-ignore lint/suspicious/noArrayIndexKey: this function's own doc
    <Fragment key={index}>{render(item)}</Fragment>
  ));
}

function renderBlocks(blocks: readonly Block[]): ReactNode[] {
  return sequence(blocks, renderBlock);
}

function renderBlock(block: Block): ReactNode {
  // The mark a block wears when the QUOTE was not found: the whole block is the
  // answer then, and it is a different claim from a marked phrase — "the
  // passage is somewhere in here" rather than "these words".
  const marked: MarkAttr =
    block.marked === true ? { "data-markdown-mark": "" } : {};
  if (block.kind === "heading") return renderHeading(block.level, block.run);
  if (block.kind === "paragraph") {
    return <p {...marked}>{renderRun(block.run)}</p>;
  }
  if (block.kind === "rule") return <hr {...marked} />;
  if (block.kind === "code") {
    return (
      <pre className="md-code" {...marked}>
        <code>{renderRun(block.run)}</code>
      </pre>
    );
  }
  if (block.kind === "quote") {
    return <blockquote {...marked}>{renderBlocks(block.blocks)}</blockquote>;
  }
  if (block.kind === "list") {
    return renderList(block.ordered, block.items, marked);
  }
  return renderTable(block.header, block.rows, marked);
}

/** The one attribute a block wears, spelled as a type so nothing else can ride
 *  in on the spread that applies it. */
type MarkAttr = { "data-markdown-mark"?: string };

/** The heading levels, written out: a tag name built from a number is a string
 *  that becomes an element, which is the one thing this file does not do. */
const HEADINGS = ["h1", "h2", "h3", "h4", "h5", "h6"] as const;

function renderHeading(level: number, run: Run): ReactNode {
  const Tag = HEADINGS[Math.min(Math.max(level, 1), 6) - 1];
  return <Tag>{renderRun(run)}</Tag>;
}

function renderList(
  ordered: boolean,
  items: readonly Run[],
  marked: MarkAttr,
): ReactNode {
  const rendered = sequence(items, (item) => <li>{renderRun(item)}</li>);
  return ordered ? (
    <ol {...marked}>{rendered}</ol>
  ) : (
    <ul {...marked}>{rendered}</ul>
  );
}

function renderTable(
  header: readonly Run[],
  rows: readonly (readonly Run[])[],
  marked: MarkAttr,
): ReactNode {
  // The region's name is the table's OWN header row. A `label` prop would be
  // copy in a primitive, and a generic one ("Table") tells a reader arriving on
  // the tab stop nothing about which of a document's tables they have reached.
  const label = header.map((cell) => runText(cell.nodes)).join(", ");
  return (
    // A corpus table is as wide as its author made it and this column is not,
    // so it goes in the house's ONE overflow box: `TableScroll` takes a tab
    // stop and announces itself as a named region only while it is actually
    // holding something past its right edge, which is the whole difference
    // between a mouse reaching those columns and everybody reaching them.
    <TableScroll label={label}>
      <table {...marked}>
        <thead>
          <tr>
            {sequence(header, (cell) => (
              <th>{renderRun(cell)}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sequence(rows, (row) => (
            <tr>
              {sequence(row, (cell) => (
                <td>{renderRun(cell)}</td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </TableScroll>
  );
}

function renderRun(run: Run): ReactNode[] {
  return sequence(run.nodes, renderInline);
}

function renderInline(node: Inline): ReactNode {
  if (node.kind === "text") return node.text;
  if (node.kind === "code") return <code>{node.text}</code>;
  if (node.kind === "strong") return <strong>{children(node.children)}</strong>;
  if (node.kind === "em") return <em>{children(node.children)}</em>;
  if (node.kind === "mark") return <mark>{children(node.children)}</mark>;
  return (
    <a
      href={node.href}
      // An outbound document link leaves this pane rather than replacing the
      // answer the reader is holding it against. `noopener` is not optional on
      // a target from a customer-uploaded file: without it the opened page can
      // steer this one.
      target={node.external ? "_blank" : undefined}
      rel="noopener noreferrer"
    >
      {children(node.children)}
    </a>
  );
}

function children(nodes: readonly Inline[]): ReactNode[] {
  return renderRun({ nodes: [...nodes] });
}
