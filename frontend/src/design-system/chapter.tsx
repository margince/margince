// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";

import { Card } from "./atoms";
import { Heading } from "./heading";
import { useJumpTo } from "./jumpbar";
import { FlatPanelHeads } from "./panel";
import "./chapter.css";

// A long record tab reads as a few chapters in a fixed order. Every chapter
// has the same anatomy, so a reader learns the rhythm once and scans the rest:
// a title, the one sentence that answers it, the chapter's verb, then one card.
export function Chapter({
  id,
  title,
  answer,
  action,
  rows,
  children,
}: Readonly<{
  // The anchor a ChapterJump chip scrolls to.
  id: string;
  title: string;
  // The reading the chapter stands on, in one sentence. It is the answer to
  // the title, so a reader who stops here has what they came for.
  answer?: ReactNode;
  action?: ReactNode;
  // Each panel in the card becomes a row: its name on the left, its body on
  // the right. For a chapter of small, unlike sections such as a record's
  // permissions, lists and settings.
  rows?: boolean;
  // Absent when the answer is the whole chapter: an empty card reads as a
  // section that failed to load.
  children?: ReactNode;
}>) {
  const titleId = useId();
  return (
    <section
      className="chapter"
      id={id}
      tabIndex={-1}
      aria-labelledby={titleId}
    >
      <div className="chapter-top">
        <div className="chapter-lead">
          <Heading size="medium" as="h2" id={titleId}>
            {title}
          </Heading>
          {answer ? <p className="chapter-answer">{answer}</p> : null}
        </div>
        {action ? <div className="chapter-action">{action}</div> : null}
      </div>
      {children ? (
        <Card as="div" className="chapter-card">
          <div className="chapter-parts" data-layout={rows ? "rows" : "stack"}>
            <FlatPanelHeads value>{children}</FlatPanelHeads>
          </div>
        </Card>
      ) : null}
    </section>
  );
}

// The row of chips over a tab's chapters: each names a chapter and its reading,
// and moves focus there.
export function ChapterJump({
  label,
  chapters,
}: Readonly<{
  label: string;
  chapters: ReadonlyArray<{ id: string; title: string; reading?: string }>;
}>) {
  const jumpTo = useJumpTo();
  return (
    <nav className="chapter-jump" aria-label={label}>
      {chapters.map((c) => (
        <button
          key={c.id}
          type="button"
          className="chapter-jump-chip"
          onClick={() => jumpTo(c.id)}
        >
          <span className="chapter-jump-title">{c.title}</span>
          {c.reading ? (
            <span className="chapter-jump-reading">{c.reading}</span>
          ) : null}
        </button>
      ))}
    </nav>
  );
}

// The chapters of one tab, at one interval.
export function Chapters({ children }: Readonly<{ children: ReactNode }>) {
  return <div className="chapters">{children}</div>;
}
