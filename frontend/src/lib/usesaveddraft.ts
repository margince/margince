// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * The draft of an input that saves when the reader leaves it.
 *
 * The saved value replaces the draft only once every save it sent has
 * settled and nothing newer was typed. So an earlier answer never erases a
 * newer draft.
 *
 * MoneyInput and NumberSetting do not decide which of draft and saved value
 * wins, so this hook sits beside them.
 */
export function useSavedDraft<Draft, Saved>({
  saved,
  toDraft,
  submit,
}: Readonly<{
  saved: Saved;
  toDraft: (saved: Saved) => Draft;
  // Sends the draft when it differs from the saved value. The promise settles
  // with the request; nothing is returned when nothing was sent.
  submit: (draft: Draft) => Promise<unknown> | undefined;
}>) {
  const [draft, setDraft] = useState(() => toDraft(saved));
  const [unsaved, setUnsaved] = useState(false);
  const [settled, setSettled] = useState(0);
  const edited = useRef(false);
  const left = useRef(false);
  const failed = useRef(false);
  const pending = useRef(0);
  const sent = useRef<Draft | undefined>(undefined);
  const seen = useRef(saved);
  const current = useRef(saved);
  current.current = saved;
  const latest = useRef({ draft, toDraft, submit });
  latest.current = { draft, toDraft, submit };

  const send = useCallback((value: Draft) => {
    const request = latest.current.submit(value);
    if (request === undefined) {
      return;
    }
    sent.current = value;
    pending.current += 1;
    request.then(
      () => {
        pending.current -= 1;
        failed.current = false;
        setUnsaved(false);
        setSettled((count) => count + 1);
      },
      () => {
        pending.current -= 1;
        failed.current = true;
        setUnsaved(true);
      },
    );
  }, []);

  useEffect(() => {
    // `settled` re-runs this once a save answers, with the saved value that
    // answer wrote.
    void settled;
    if (pending.current > 0 || failed.current) {
      return;
    }
    const now = latest.current;
    if (!edited.current) {
      if (!Object.is(seen.current, saved)) {
        seen.current = saved;
        setDraft(now.toDraft(saved));
      }
      return;
    }
    if (Object.is(now.draft, sent.current)) {
      edited.current = false;
      sent.current = undefined;
      seen.current = saved;
      setDraft(now.toDraft(saved));
      return;
    }
    // The reader left the box with newer text while an earlier save was
    // pending, and that save has now moved the saved value under it.
    if (left.current) {
      send(now.draft);
    }
  }, [saved, settled, send]);

  const edit = useCallback((next: Draft) => {
    edited.current = true;
    left.current = false;
    failed.current = false;
    setUnsaved(false);
    setDraft(next);
  }, []);

  const leave = useCallback(
    (value?: Draft) => {
      left.current = true;
      send(value === undefined ? latest.current.draft : value);
    },
    [send],
  );

  // Drops the draft for the saved value, for a box left empty.
  const restore = useCallback(() => {
    edited.current = false;
    failed.current = false;
    setUnsaved(false);
    setDraft(latest.current.toDraft(current.current));
  }, []);

  return { draft, edit, leave, restore, unsaved };
}
