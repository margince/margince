import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { useAnchoredToTrigger } from "./anchored";
import { Button } from "./atoms";
import { useHoverIntent } from "./hoverintent";
import type { ConfidenceLevel, Provenance } from "./trust";
import { ProvenanceTag } from "./trust";
import "./evidencemark.css";

// The ONE provenance affordance (design-language §4).
//
// A field that came from somewhere other than a contact typing it carries a
// dotted underline. Opening the mark says where it came from, how sure the
// system was, the text it was read from, and when — with a way through to
// the full history of that field.
//
// It replaces the stack of chips that used to sit under every value
// (provenance tag + confidence meter + evidence chip, three widgets per
// field). Three chips under a value do not read as "this was derived"; they
// read as clutter, and the value they describe gets lost among them. One
// mark on the value itself keeps the record readable and puts the receipts
// one interaction away — the same information, on demand rather than always.

// openMark is the one panel currently showing, across every mark on the
// page. Opening one closes the last, so a keyboard user tabbing along a
// column of marked values leaves a trail of one panel rather than a stack of
// overlapping regions — the same behaviour pointer dismissal gives, made
// true for every input method rather than assumed.
let closeOpenMark: (() => void) | null = null;

// The first thing in the panel a reader can land on. The same set popover.tsx
// keeps for its own portalled panel, kept here rather than exported from one —
// this is a CSS selector, not a shared rule about focus.
const FOCUSABLE =
  'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])';

export type EvidenceMarkSource = {
  provenance: Provenance;
  confidence?: ConfidenceLevel;
  snippet?: string | null;
  sourceUrl?: string | null;
  at?: string | null;
};

export function EvidenceMark({
  value,
  source,
  onOpenHistory,
  historyLabel,
}: Readonly<{
  // The rendered value the mark is about. A mark with nothing to explain
  // renders as plain text: an underline that opens an empty popover teaches
  // the reader to stop opening them.
  value: string;
  source?: EvidenceMarkSource;
  onOpenHistory?: () => void;
  historyLabel?: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // How the panel came to be open. A press is a reader asking for it, and
  // focus follows into the panel's own controls (the "Full history" button);
  // a passing pointer is not, and moving focus off whatever the reader was
  // doing to put it in a panel they merely settled on would be the page
  // grabbing at them. The same distinction popover.tsx draws for its own
  // portalled panel.
  const [openedBy, setOpenedBy] = useState<"press" | "hover">("press");
  const panelId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLElement>(null);
  // The receipt opens under a pointer that has SETTLED on the value, and
  // closes once the pointer has left both the value and the receipt: a claim
  // is checked by resting on it, not by clicking through to it. The click
  // still toggles, for a touch screen and for a reader who wants it to stay.
  const hover = useHoverIntent(
    () => {
      setOpenedBy("hover");
      setOpen(true);
    },
    () => setOpen(false),
  );
  // Moves focus into the panel once it is open, but only when a press opened
  // it and only when there is a control in it to land on (a receipt with no
  // "Full history" link has nothing to focus, and a screen reader is already
  // on the trigger's own accessible name). Portalled to the body, the panel
  // is no longer the trigger's next DOM sibling, so Tab no longer reaches it
  // by adjacency the way it did before the portal — this is what restores
  // that reach.
  useEffect(() => {
    if (!open || openedBy === "hover") {
      return;
    }
    panelRef.current?.querySelector<HTMLElement>(FOCUSABLE)?.focus();
  }, [open, openedBy]);
  // The receipt is portalled to the body and placed against the trigger's own
  // rectangle (anchored.ts), the same reason the popover is (popover.tsx): a
  // value near the bottom of a `Panel` sits inside `overflow: hidden`, and a
  // panel positioned relative to that value was clipped at the panel's edge
  // rather than reaching the reader.
  const at = useAnchoredToTrigger(open, triggerRef, panelRef, "start");

  // Registered while this mark is the open one, and cleared on close or
  // unmount so a removed mark never leaves a closer pointing at a component
  // that is gone.
  useEffect(() => {
    if (!open) {
      return;
    }
    const close = () => setOpen(false);
    closeOpenMark?.();
    closeOpenMark = close;
    return () => {
      if (closeOpenMark === close) {
        closeOpenMark = null;
      }
    };
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        // Focus returns to what opened the panel, so Escape does not drop
        // the reader at the top of the document.
        triggerRef.current?.focus();
      }
    };
    const onPointer = (event: MouseEvent) => {
      const target = event.target;
      // A click outside the DOM tree (or on something that is not a node at
      // all) closes the panel: it is certainly not inside it.
      if (!(target instanceof Node)) {
        setOpen(false);
        return;
      }
      if (
        !panelRef.current?.contains(target) &&
        !triggerRef.current?.contains(target)
      ) {
        setOpen(false);
      }
    };
    globalThis.addEventListener("keydown", onKey);
    globalThis.addEventListener("mousedown", onPointer);
    return () => {
      globalThis.removeEventListener("keydown", onKey);
      globalThis.removeEventListener("mousedown", onPointer);
    };
  }, [open]);

  if (!source) {
    return <span>{value}</span>;
  }

  return (
    <span className="evmark" {...hover}>
      <button
        ref={triggerRef}
        type="button"
        className="evmark-trigger"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        aria-label={t("evidence.explain", { value })}
        onClick={() => {
          setOpenedBy("press");
          setOpen((was) => !was);
        }}
      >
        {value}
      </button>
      {open &&
        createPortal(
          // A named section rather than a dialog: this is a disclosure beside
          // the value, not a modal — the page behind it stays usable and
          // nothing here traps focus. The accessible name makes it a landmark
          // a screen reader can jump to, and only one is ever open.
          <section
            ref={panelRef}
            id={panelId}
            className="evmark-panel"
            aria-label={t("evidence.explain", { value })}
            // Portalled to the body (below), so it is no longer a DOM
            // descendant of `.evmark` — pointer enter/leave stop tracking
            // containment once the panel moves outside that subtree, and
            // without its own copy of the hover pair a pointer crossing from
            // the trigger into the receipt read as leaving both, closing the
            // panel a reader was moving toward. The same reason popover.tsx's
            // portalled panel carries the same pair.
            {...hover}
            style={{
              top: `${at.top}px`,
              left: `${at.left}px`,
              maxHeight: `${at.maxHeight}px`,
            }}
          >
            <p className="evmark-row">
              <ProvenanceTag provenance={source.provenance} />
              {source.confidence && (
                <span className="evmark-confidence">
                  {t(`confidence.${source.confidence}`)}
                </span>
              )}
            </p>
            {source.snippet && (
              <blockquote className="evmark-snippet">
                {source.snippet}
              </blockquote>
            )}
            {source.sourceUrl && (
              <p className="evmark-source">{source.sourceUrl}</p>
            )}
            {source.at && <p className="evmark-at">{source.at}</p>}
            {onOpenHistory && (
              <Button
                small
                onClick={() => {
                  setOpen(false);
                  onOpenHistory();
                }}
              >
                {historyLabel ?? t("evidence.fullHistory")}
              </Button>
            )}
          </section>,
          document.body,
        )}
    </span>
  );
}
