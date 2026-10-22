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
 * A hook, because the draft sits several components below the guard. Outside
 * a provider (a story, a test) it claims nothing.
 */
export function useUnsavedGuard(dirty: boolean, scope?: string): void {
  const { claim, release } = useContext(UnsavedContext) ?? {};
  // One token per mounting, so two rows of the same card are two claims.
  const token = useId();

  useEffect(() => {
    claim?.(token, dirty, scope);
  }, [claim, token, dirty, scope]);

  // A claim left after unmount would guard a draft that no longer exists.
  useEffect(() => () => release?.(token), [release, token]);
}
