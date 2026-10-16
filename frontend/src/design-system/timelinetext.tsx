import type { ReactNode } from "react";
import { useLayoutEffect, useMemo, useState } from "react";
import { splitEmailBody } from "../format/emailtext";
import { useT } from "../i18n";
import { writtenAsMarkdown } from "./activitytimeline";
import { Markdown } from "./markdown";
import { URL_PATTERN } from "./markdown-autolink";
import "./composed.css";

import type { TimelineEntry } from "./timelineentry";

function linkify(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(URL_PATTERN)) {
    const at = match.index;
    if (at > last) {
      out.push(text.slice(last, at));
    }
    out.push(
      <a
        key={`${at}-${match[0]}`}
        href={match[0]}
        target="_blank"
        rel="noopener noreferrer nofollow"
        className="tl-text-link"
      >
        {match[0]}
      </a>,
    );
    last = at + match[0].length;
  }
  if (last < text.length) {
    out.push(text.slice(last));
  }
  return out;
}

/**
 * TimelineText is the message itself, three lines by default and the whole of
 * it on request. A timeline of full emails would be a mailbox.
 *
 * On a mail row the sign-off and quoted history fold into a second control,
 * because the split is a heuristic and may take too much. Only a mail is split,
 * since a note may say "Viele Grüße" as prose. What a rep writes (a note, call,
 * meeting or task) is markdown; a mail, a chat and a transcript stay text.
 */
export function TimelineText({
  text,
  entry,
}: Readonly<{
  text: string;
  entry: Pick<TimelineEntry, "kind" | "transcript">;
}>) {
  const t = useT();
  const email = entry.kind === "email";
  const [open, setOpen] = useState(false);
  const [tailOpen, setTailOpen] = useState(false);
  // Measured, not guessed: the clamp is three visual lines at the column's
  // width. A short guess leaves clipped text with no control to reach it.
  const [clipped, setClipped] = useState(false);
  const [el, setEl] = useState<HTMLElement | null>(null);
  // Markdown is block content, which a <span> may not hold.
  const Block = writtenAsMarkdown(entry) ? "div" : "span";
  // The tail is what the reader is spared. `trimmed` is what the row shows and
  // what the clamp measures, so the split must happen before that effect.
  const parts = useMemo(
    () => (email ? splitEmailBody(text) : null),
    [email, text],
  );
  const trimmed = parts
    ? [parts.header, parts.main].filter(Boolean).join("\n\n")
    : text.trim();
  const tail = parts?.trimmed ?? "";
  // A row is keyed by activity id, so this stays mounted when its entry is
  // replaced. The next mail's signature stays closed.
  const [shownFor, setShownFor] = useState(text);
  if (shownFor !== text) {
    setShownFor(text);
    setTailOpen(false);
  }

  useLayoutEffect(() => {
    // Nothing to measure while expanded: scrollHeight equals clientHeight, and
    // re-measuring there would drop the control that collapses it again. Empty
    // text renders nothing, so there is nothing that could be clipped.
    if (!el || open || !trimmed) {
      return;
    }
    const measure = () => setClipped(el.scrollHeight > el.clientHeight + 1);
    measure();
    // The column is resizable, so a width change can start or stop the
    // clipping. Guarded because jsdom has no ResizeObserver.
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [el, open, trimmed]);

  if (!trimmed) {
    return null;
  }
  return (
    <Block className="tl-text">
      <Block ref={setEl} className={open ? "tl-text-full" : "tl-text-clamp"}>
        {Block === "div" ? (
          <Markdown source={trimmed} autolink />
        ) : (
          linkify(trimmed)
        )}
      </Block>
      {(clipped || open) && (
        <button
          type="button"
          className="tl-text-toggle"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          {open ? t("timeline.textLess") : t("timeline.textMore")}
        </button>
      )}
      {/* Offered whenever something was folded away, not only once the message
          is expanded: a mail short enough never to clip still has a signature,
          and gating this on the clamp would put that text out of reach. */}
      {tail && (
        <>
          <button
            type="button"
            className="tl-text-toggle tl-text-tail-toggle"
            aria-expanded={tailOpen}
            onClick={() => setTailOpen(!tailOpen)}
          >
            {tailOpen ? t("timeline.tailLess") : t("timeline.tailMore")}
          </button>
          {tailOpen && <span className="tl-text-tail">{linkify(tail)}</span>}
        </>
      )}
    </Block>
  );
}
