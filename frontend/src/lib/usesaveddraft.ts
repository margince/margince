// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * The draft of an input that saves when the reader leaves it.
 *
 * The cell keeps one identity through a save, so a newer edit survives the
 * answer of an earlier one. The saved value replaces the draft only when the
 * reader has typed nothing past what was last sent.
 */
export function useSavedDraft<Draft, Saved>({
  saved,
  toDraft,
  submit,
}: Readonly<{
  saved: Saved;
  toDraft: (saved: Saved) => Draft;
  // Sends the draft if it differs from the saved value; may refuse to.
  submit: (draft: Draft) => void;
}>) {
  const [draft, setDraft] = useState(() => toDraft(saved));
  const edited = useRef(false);
  const left = useRef(false);
  const sent = useRef<Draft | undefined>(undefined);
  const latest = useRef({ draft, toDraft, submit });
  latest.current = { draft, toDraft, submit };
  const seen = useRef(saved);

  useEffect(() => {
    if (Object.is(seen.current, saved)) {
      return;
    }
    seen.current = saved;
    const now = latest.current;
    if (!edited.current || Object.is(now.draft, sent.current)) {
      edited.current = false;
      setDraft(now.toDraft(saved));
      return;
    }
    // The reader already left the box with this newer text, and an earlier
    // answer moved the saved value under it, so the text is still unsent.
    if (left.current) {
      sent.current = now.draft;
      now.submit(now.draft);
    }
  }, [saved]);

  const edit = useCallback((next: Draft) => {
    edited.current = true;
    left.current = false;
    setDraft(next);
  }, []);

  const leave = useCallback((value?: Draft) => {
    const now = latest.current;
    const leaving = value === undefined ? now.draft : value;
    left.current = true;
    sent.current = leaving;
    now.submit(leaving);
  }, []);

  // Drops the draft for the saved value, for a box left empty.
  const restore = useCallback(() => {
    const now = latest.current;
    edited.current = false;
    setDraft(now.toDraft(seen.current));
  }, []);

  return { draft, edit, leave, restore };
}
