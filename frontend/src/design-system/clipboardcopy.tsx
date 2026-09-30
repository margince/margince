// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactElement, useRef, useState } from "react";
import { useT } from "../i18n";
import { Callout } from "./callout";

/**
 * Putting one string on the reader's clipboard, and saying so when the browser
 * will not take it.
 *
 * Eight screens wrote this out by hand and no two agreed. Seven guarded the API
 * object and one did not; of the seven, one guarded it and then returned in
 * silence. The notice was a `Callout` on five, a tinted `span` on one, a bare
 * paragraph on another and nothing at all on the last — so what a reader on a plain-http
 * installation learned depended on which screen they happened to be standing on.
 *
 * A hook rather than a `CopyButton`, for the reason `usePasswordReveal` is one:
 * the label and the failure are one fact, and a caller handed only the button
 * would hold the other half itself and be able to get the two out of step.
 * There is also no button to hand some of them — `analytics.share.tsx` passes
 * its label to `ConfirmModal`, which draws the control itself.
 *
 * The TEXT is the hook's argument rather than the copy call's, which is what
 * makes `copied` a claim about something true: the state records WHAT was
 * copied, so a draft edited after copying, or a YAML re-rendered under a new
 * filename, stops saying Copied without its screen having to remember to say so.
 *
 * `copied` is a claim about THIS control, and the clipboard is one slot: a
 * surface drawing several copy controls over it owns which of them is current,
 * because two buttons both reading Copied tell a reader the clipboard holds
 * both. `onCopied` is the hand-off for that — it fires when the write actually
 * lands, never on the press.
 *
 * Copy is the caller's, translated, as everywhere in this directory — except
 * the failure's first line, which is one shared key. That sentence is about the
 * BROWSER rather than about the link, the secret or the agenda, so it is the
 * same fact on every screen, and eight callers passing it in is exactly how the
 * eight wordings this replaces came about. What differs is the way out, and
 * that arrives as `remedy`.
 */
export function useClipboardCopy(
  text: string,
  labels: Readonly<{ copy: string; copied: string; remedy: string }>,
  onCopied?: () => void,
): Readonly<{
  label: string;
  copied: boolean;
  copy: () => void;
  notice: ReactElement | null;
}> {
  const t = useT();
  const [written, setWritten] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  // Which attempt the reader is waiting on. Two presses are two independent
  // writes that can settle in either order, and an older one landing last would
  // speak for the newer: a rejection arriving after a success draws the failure
  // notice over a copy that worked.
  const attempt = useRef(0);
  const copied = written === text;

  function copy() {
    // `navigator.clipboard` is UNDEFINED outside a secure context, so a missing
    // clipboard throws on the property access rather than rejecting, and a bare
    // `.writeText(…).catch(…)` never reaches its handler. An email-less
    // installation served over plain http is that deployment, not an edge case.
    const mine = ++attempt.current;
    const writer = navigator.clipboard;
    if (!writer) {
      // Dropping the claim as the rejection branch does: a control that had
      // copied and then met a browser with no clipboard would otherwise draw
      // Copied and the danger notice at the same time.
      setWritten(null);
      setFailed(true);
      return;
    }
    writer.writeText(text).then(
      () => {
        if (mine !== attempt.current) return;
        setWritten(text);
        setFailed(false);
        onCopied?.();
      },
      () => {
        if (mine !== attempt.current) return;
        setWritten(null);
        setFailed(true);
      },
    );
  }

  return {
    label: copied ? labels.copied : labels.copy,
    copied,
    copy,
    notice: failed ? (
      <Callout
        tone="danger"
        kind="outcome"
        title={t("clipboard.copyFailedTitle")}
      >
        {labels.remedy}
      </Callout>
    ) : null,
  };
}
