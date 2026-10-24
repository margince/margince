// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from "react";
import { useT } from "../i18n";
import { ActionRow } from "./actionrow";
import { Button } from "./atoms";
import { ErrorLine } from "./errorline";
import { SaveBar } from "./savebar";

/** One card's draft, as the page's save bar sees it. */
export type SaveDraft = Readonly<{
  /** True while the card's fields differ from what was last saved. */
  dirty: boolean;
  /** True while a field says why its value cannot be sent. */
  invalid?: boolean;
  /** A default shown as the answer: saveable, yet leaving asks nothing. */
  offered?: boolean;
  /** Writes the draft; a rejection keeps it and is shown on the bar. */
  save: () => Promise<unknown>;
  /** Puts the card back to what was last saved. */
  discard: () => void;
}>;

/** The page's drafts taken together: what the bar and the leave guard read. */
export type SavePage = Readonly<{
  /** Some card holds an edit: what the leave guard asks about. */
  dirty: boolean;
  /** Some card holds an edit or an offered default: what the bar is drawn for. */
  open: boolean;
  invalid: boolean;
  pending: boolean;
  error: unknown;
  /** Resolves true once every waiting draft is written. */
  save: () => Promise<boolean>;
  discard: () => void;
}>;

type Mark = Readonly<{ dirty: boolean; invalid: boolean; offered: boolean }>;

const waiting = (draft: SaveDraft | Mark) =>
  draft.dirty || (draft.offered ?? false);

type Registry = Readonly<{
  page: SavePage;
  register: (token: string, draft: SaveDraft) => void;
  mark: (token: string, next: Mark) => void;
  release: (token: string) => void;
}>;

const SavePageContext = createContext<Registry | null>(null);

/**
 * Collects every card's draft on one page so a single bar saves them together.
 * The drafts' callbacks live in a ref. Inline closures change on every render,
 * so holding them in state would re-render the page in a loop.
 */
export function SavePageProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  const drafts = useRef(new Map<string, SaveDraft>());
  const [marks, setMarks] = useState<ReadonlyMap<string, Mark>>(new Map());
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const register = useCallback((token: string, draft: SaveDraft) => {
    drafts.current.set(token, draft);
  }, []);
  const mark = useCallback((token: string, next: Mark) => {
    setMarks((held) => {
      const was = held.get(token);
      if (
        was?.dirty === next.dirty &&
        was.invalid === next.invalid &&
        was.offered === next.offered
      ) {
        return held;
      }
      return new Map(held).set(token, next);
    });
  }, []);
  const release = useCallback((token: string) => {
    drafts.current.delete(token);
    setMarks((held) => {
      if (!held.has(token)) return held;
      const shrunk = new Map(held);
      shrunk.delete(token);
      return shrunk;
    });
  }, []);

  const save = useCallback(async () => {
    const due = [...drafts.current.values()].filter(waiting);
    if (due.some((draft) => draft.invalid)) return false;
    setPending(true);
    setError(null);
    // Settled rather than all: one refused card must not stop the others.
    const results = await Promise.allSettled(due.map((draft) => draft.save()));
    setPending(false);
    const refused = results.find((result) => result.status === "rejected");
    if (refused) {
      setError(refused.reason);
      return false;
    }
    return true;
  }, []);
  const discard = useCallback(() => {
    for (const draft of drafts.current.values()) {
      if (waiting(draft)) draft.discard();
    }
    setError(null);
  }, []);

  const states = [...marks.values()];
  const dirty = states.some((state) => state.dirty);
  const open = states.some(waiting);
  const invalid = states.some((state) => waiting(state) && state.invalid);
  // A refusal belongs to the drafts it failed for; once none is left, nor is it.
  if (!open && error !== null) {
    setError(null);
  }

  const registry = useMemo<Registry>(
    () => ({
      page: { dirty, open, invalid, pending, error, save, discard },
      register,
      mark,
      release,
    }),
    [
      dirty,
      open,
      invalid,
      pending,
      error,
      save,
      discard,
      register,
      mark,
      release,
    ],
  );
  return (
    <SavePageContext.Provider value={registry}>
      {children}
    </SavePageContext.Provider>
  );
}

/**
 * Hands a card's draft to the page's save bar. Returns whether a page holds it:
 * outside a `SavePageProvider` nothing collects the draft, and the caller keeps
 * whatever commit it had of its own.
 */
export function useSaveDraft(draft: SaveDraft): boolean {
  const registry = useContext(SavePageContext);
  const token = useId();
  const register = registry?.register;
  const mark = registry?.mark;
  const release = registry?.release;
  const invalid = draft.invalid ?? false;
  const offered = draft.offered ?? false;

  // Every render, so the bar's Save always calls the latest closure.
  useEffect(() => {
    register?.(token, draft);
  });
  useEffect(() => {
    mark?.(token, { dirty: draft.dirty, invalid, offered });
  }, [mark, token, draft.dirty, invalid, offered]);
  useEffect(() => () => release?.(token), [release, token]);
  return registry !== null;
}

/** The page's drafts taken together, or null outside a `SavePageProvider`. */
export function useSavePage(): SavePage | null {
  return useContext(SavePageContext)?.page ?? null;
}

/**
 * The one Save of a settings page, pinned to the foot of the view. It is drawn
 * only while some card on the page holds a draft.
 */
export function PageSaveBar() {
  const t = useT();
  const page = useSavePage();
  if (!page?.open) return null;
  return (
    <SaveBar label={t("saveBar.unsaved")}>
      <ActionRow
        primary={
          <Button
            variant="primary"
            pending={page.pending}
            reason={page.invalid ? t("saveBar.invalid") : undefined}
            onClick={() => {
              void page.save();
            }}
          >
            {t("saveBar.save")}
          </Button>
        }
      >
        <p className="t-caption save-bar-caption">{t("saveBar.unsaved")}</p>
        {/* A discard that lands while the save runs would race it. */}
        <Button variant="ghost" disabled={page.pending} onClick={page.discard}>
          {t("saveBar.discard")}
        </Button>
      </ActionRow>
      <ErrorLine error={page.error} />
    </SaveBar>
  );
}
