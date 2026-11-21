// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

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
  const [draft, setDraftState] = useState(() => toDraft(saved));
  // Kept as the draft changes, so a blur right after a keystroke sends it.
  const draftRef = useRef(draft);
  const setDraft = useCallback((next: Draft) => {
    draftRef.current = next;
    setDraftState(next);
  }, []);
  const [unsaved, setUnsaved] = useState(false);
  const [settled, setSettled] = useState(0);
  const edited = useRef(false);
  const left = useRef(false);
  const failed = useRef(false);
  const pending = useRef(0);
  const sent = useRef<Draft | undefined>(undefined);
  const lastSaved = useRef(saved);
  const moved = useRef(false);
  const current = useRef(saved);
  const latest = useRef({ toDraft, submit });
  // Layout effects run before the passive effect below reads these refs.
  useLayoutEffect(() => {
    current.current = saved;
    latest.current = { toDraft, submit };
  });

  const send = useCallback((value: Draft) => {
    const request = latest.current.submit(value);
    if (request === undefined) {
      return;
    }
    sent.current = value;
    moved.current = false;
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
    // `settled` re-runs this once a save answers. The answer can reach here
    // before the saved value it wrote reaches the props, so the draft is only
    // replaced after the saved value itself has moved.
    void settled;
    if (!Object.is(lastSaved.current, saved)) {
      lastSaved.current = saved;
      moved.current = true;
    }
    if (pending.current > 0 || failed.current || !moved.current) {
      return;
    }
    const now = latest.current;
    if (!edited.current || Object.is(draftRef.current, sent.current)) {
      moved.current = false;
      edited.current = false;
      sent.current = undefined;
      setDraft(now.toDraft(saved));
      return;
    }
    // The reader left the box with newer text while an earlier save was
    // pending, and that save has now moved the saved value under it.
    if (left.current) {
      moved.current = false;
      send(draftRef.current);
    }
  }, [saved, settled, send, setDraft]);

  const edit = useCallback(
    (next: Draft) => {
      edited.current = true;
      left.current = false;
      failed.current = false;
      setUnsaved(false);
      setDraft(next);
    },
    [setDraft],
  );

  const leave = useCallback(
    (value?: Draft) => {
      left.current = true;
      send(value === undefined ? draftRef.current : value);
    },
    [send],
  );

  // Drops the draft for the saved value, for a box left empty.
  const restore = useCallback(() => {
    edited.current = false;
    failed.current = false;
    setUnsaved(false);
    setDraft(latest.current.toDraft(current.current));
  }, [setDraft]);

  return { draft, edit, leave, restore, unsaved };
}
