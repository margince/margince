import { Fragment } from "react";
import { Logomark } from "../../design-system/logomark";
import { type Translator, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { ThreadEntry } from "./conversation-machine";
import "./conversation.css";

// Presentational pieces for the conversation. Copy always resolves through the
// i18n catalogs; server-derived params arrive as data and render verbatim, while
// paramKeys are translated here.

type NarrationEntry = Extract<ThreadEntry, { kind: "narration" }>;

function resolvedParams(
  t: Translator,
  params: Record<string, string> | undefined,
  paramKeys: Record<string, MessageKey> | undefined,
): Record<string, string> {
  const translated = Object.fromEntries(
    Object.entries(paramKeys ?? {}).map(([name, key]) => [name, t(key)]),
  );
  return { ...params, ...translated };
}

// Word-by-word reveal for narration that arrives LIVE — speech gets a beat,
// factual cards (questions, outcomes, user turns) never do. The animated copy
// is presentation only (aria-hidden, per-word spans); the full sentence rides
// along visually hidden so assistive tech and text queries always see one
// coherent string. The stagger shrinks with word count so a long sentence
// finishes inside the same cap as a short one; prefers-reduced-motion
// collapses the animation entirely (conversation.css).

const REVEAL_WORD_STEP_MS = 90;
const REVEAL_TOTAL_CAP_MS = 1200;

export function RevealText({ text }: Readonly<{ text: string }>) {
  const words = text.split(/\s+/).filter((word) => word !== "");
  const step = Math.min(
    REVEAL_WORD_STEP_MS,
    REVEAL_TOTAL_CAP_MS / Math.max(1, words.length),
  );
  // Repeated words need distinct keys; an occurrence counter keeps them
  // stable without keying on the array index.
  const seen = new Map<string, number>();
  return (
    <>
      <span className="ob-conv-reveal-source">{text}</span>
      <span className="ob-conv-reveal" aria-hidden>
        {words.map((word, position) => {
          const occurrence = (seen.get(word) ?? 0) + 1;
          seen.set(word, occurrence);
          return (
            <Fragment key={`${word}:${occurrence}`}>
              <span
                style={{ animationDelay: `${Math.round(position * step)}ms` }}
              >
                {word}
              </span>{" "}
            </Fragment>
          );
        })}
      </span>
    </>
  );
}

// Matches the pulse animation length in conversation.css.
const JUMP_PULSE_MS = 1600;

// The contract a collapsed row opts into: dispatched at the exact element
// carrying `data-finding-id` before this function looks for anything to
// focus inside it, so a row that renders its control only once expanded
// (confirm-card.tsx's FieldRow) gets the chance to open first. A row that
// never listens — settled rows, a contact or fact entry with nothing to
// edit — simply ignores it, and the fallback below still focuses the row
// itself.
export const FINDING_EXPAND_EVENT = "ob:expand-finding";

// How long this function is willing to wait for a row's control to appear
// after asking it to expand, before it gives up and focuses the row
// instead — long enough for a state update and re-render, never so long
// the jump feels stuck.
const EXPAND_WAIT_MS = 400;

// The jump's actual destination: the field's own input or textarea if the
// row carries one (a click on a to-do means "let me fill this in", so the
// caret belongs in the control, not merely on the row that contains it),
// falling back to the row itself for anything with nothing to type into.
function focusableControl(
  node: Element,
): HTMLInputElement | HTMLTextAreaElement | null {
  if (node instanceof HTMLInputElement || node instanceof HTMLTextAreaElement) {
    return node;
  }
  return node.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    "input, textarea",
  );
}

// Focuses the node's control the moment one exists — immediately if the row
// was already open, otherwise as soon as the expand request above causes one
// to mount. A MutationObserver rather than a fixed delay: a state update's
// timing is never guaranteed, and guessing a delay either flashes focus too
// early (nothing there yet) or leaves the human waiting past the render
// that already finished.
function focusWhenReady(node: Element): void {
  const control = focusableControl(node);
  if (control) {
    control.focus({ preventScroll: true });
    return;
  }
  if (typeof MutationObserver === "undefined") {
    // jsdom without a MutationObserver polyfill (none of this repo's tests
    // need one); in the browser it always exists.
    if (node instanceof HTMLElement) {
      node.focus({ preventScroll: true });
    }
    return;
  }
  const timeout = globalThis.setTimeout(() => {
    observer.disconnect();
    // The row never grew a control — it does not listen for the expand
    // request, or has none to offer — so the row itself is the honest
    // landing spot, exactly as before this function existed.
    if (node instanceof HTMLElement) {
      node.focus({ preventScroll: true });
    }
  }, EXPAND_WAIT_MS);
  const observer = new MutationObserver(() => {
    const found = focusableControl(node);
    if (found) {
      observer.disconnect();
      globalThis.clearTimeout(timeout);
      found.focus({ preventScroll: true });
    }
  });
  observer.observe(node, { childList: true, subtree: true });
}

/**
 * Scroll to, expand, briefly light, and focus the surface row(s) a
 * narration or a rail chip names — the "links him to the field" contract.
 * A rail control that only scrolled would leave a keyboard or screen-reader
 * user exactly where they clicked, and a collapsed row that only scrolled
 * into view would leave them focused on nothing typeable at all: the target
 * becomes where they ARE, ready to type, not just what they can see.
 * Matching is by attribute value scan (no built selector), the same choice
 * artifact.tsx documents: it needs no escaping and jsdom lacks CSS.escape.
 */
export function jumpToFindings(ids: readonly string[]): void {
  const wanted = new Set(ids);
  const nodes = [...document.querySelectorAll("[data-finding-id]")].filter(
    (node) => wanted.has(node.getAttribute("data-finding-id") ?? ""),
  );
  const first = nodes[0];
  if (first === undefined) {
    return;
  }
  first.dispatchEvent(new CustomEvent(FINDING_EXPAND_EVENT));
  const reduceMotion =
    globalThis.matchMedia?.("(prefers-reduced-motion: reduce)").matches ??
    false;
  // jsdom has no scrollIntoView; in the browser it always exists.
  first.scrollIntoView?.({
    block: "center",
    behavior: reduceMotion ? "auto" : "smooth",
  });
  focusWhenReady(first);
  for (const node of nodes) {
    node.classList.add("ob-conv-pulse");
  }
  globalThis.setTimeout(() => {
    for (const node of nodes) {
      node.classList.remove("ob-conv-pulse");
    }
  }, JUMP_PULSE_MS);
}

export function NarrationBubble({
  entry,
  reveal = false,
}: Readonly<{ entry: NarrationEntry; reveal?: boolean }>) {
  const t = useT();
  const text = t(
    entry.i18nKey,
    resolvedParams(t, entry.params, entry.paramKeys),
  );
  return (
    <div
      className="ob-conv-narration"
      data-finding-ids={entry.findingIds?.join(" ")}
    >
      {/* The product's own mark, not a letter standing in for it. `role="img"`
          with the name on the wrapper is what carries "Margince" to a screen
          reader, since the mark itself is decorative geometry. */}
      <span
        className="ob-conv-speaker"
        role="img"
        aria-label={t("ob.ai.speakerName")}
      >
        <Logomark size={16} />
      </span>
      <p>
        {reveal ? <RevealText text={text} /> : text}
        {entry.findingIds !== undefined && entry.findingIds.length > 0 && (
          // The attention contract: a message that names fields carries the
          // jump that shows and lights them on the surface.
          <button
            type="button"
            className="ob-conv-jump"
            onClick={() => jumpToFindings(entry.findingIds ?? [])}
          >
            {t("ob.conv.showField")}
          </button>
        )}
      </p>
    </div>
  );
}
