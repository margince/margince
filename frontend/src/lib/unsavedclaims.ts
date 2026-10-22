import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useState,
} from "react";

/**
 * Who holds an unsaved edit. `UnsavedGuard` keeps one registry for the page,
 * and every `Modal` keeps one for what it holds, so a dialog can ask before it
 * closes over a draft. `components/unsaved.tsx` holds what happens with a claim.
 */
export type UnsavedRegistry = Readonly<{
  /** Whether anything inside this scope has an edit that has not been written. */
  dirty: boolean;
  dirtyScopes: ReadonlySet<string>;
  claim: (token: string, dirty: boolean, scope?: string) => void;
  release: (token: string) => void;
}>;

export const UnsavedContext = createContext<UnsavedRegistry | null>(null);

type Claims = ReadonlyMap<string, Readonly<{ dirty: boolean; scope?: string }>>;

/**
 * The claims made below one surface. Each claim also reaches `parent`, so a
 * draft in a dialog still holds the page's navigation.
 */
export function useUnsavedRegistry(
  parent: UnsavedRegistry | null,
): UnsavedRegistry {
  const [claims, setClaims] = useState<Claims>(new Map());
  const parentClaim = parent?.claim;
  const parentRelease = parent?.release;

  const claim = useCallback(
    (token: string, next: boolean, scope?: string) => {
      parentClaim?.(token, next, scope);
      setClaims((held) => {
        if (
          held.get(token)?.dirty === next &&
          held.get(token)?.scope === scope
        ) {
          return held;
        }
        const grown = new Map(held);
        grown.set(token, { dirty: next, scope });
        return grown;
      });
    },
    [parentClaim],
  );

  const release = useCallback(
    (token: string) => {
      parentRelease?.(token);
      setClaims((held) => {
        if (!held.has(token)) {
          return held;
        }
        const shrunk = new Map(held);
        shrunk.delete(token);
        return shrunk;
      });
    },
    [parentRelease],
  );

  return useMemo<UnsavedRegistry>(
    () => ({
      dirty: [...claims.values()].some((entry) => entry.dirty),
      claim,
      release,
      dirtyScopes: new Set(
        [...claims.values()].flatMap((entry) =>
          entry.dirty && entry.scope ? [entry.scope] : [],
        ),
      ),
    }),
    [claims, claim, release],
  );
}

export function useHasUnsavedChanges(scope?: string): boolean {
  const registry = useContext(UnsavedContext);
  return scope
    ? (registry?.dirtyScopes.has(scope) ?? false)
    : (registry?.dirty ?? false);
}

/**
 * Declare that this component holds an unsaved edit, or no longer does.
 *
 * A hook rather than a prop chain because the draft and the guard are usually
 * several components apart — the card that owns the textarea is inside the tab
 * that is inside the screen that would hold the answer — and threading a boolean
 * up through that is how one card ends up guarded and the next one does not.
 *
 * Safe to call outside a provider: it reports nothing, which is the honest
 * behaviour for a card rendered somewhere no guard is installed (a story, a
 * test, a record page). A hook that threw there would make the provider a
 * requirement of every caller rather than of the surfaces that want guarding.
 */
export function useUnsavedGuard(dirty: boolean, scope?: string): void {
  const { claim, release } = useContext(UnsavedContext) ?? {};
  // A stable identity per mounting, so two instances of the same card — two
  // rows, two panels — are two claims rather than one that the second overwrites.
  const token = useId();

  useEffect(() => {
    claim?.(token, dirty, scope);
  }, [claim, token, dirty, scope]);

  // Released on unmount, and this is the one that has to be right: a card that
  // left the tree still holding a claim would guard the surface against a draft
  // that no longer exists, and nothing the reader can do would clear it.
  useEffect(() => () => release?.(token), [release, token]);
}
