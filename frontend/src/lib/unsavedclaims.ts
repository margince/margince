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
  /**
   * What the leave question may call to write the drafts. Empty unless every
   * dirty claim can save, so Save is offered only when it would leave nothing
   * unsaved behind.
   */
  savers: readonly Saver[];
  claim: (token: string, dirty: boolean, scope?: string, save?: Saver) => void;
  release: (token: string) => void;
}>;

/** Writes a claim's draft; resolves true once it is saved. */
export type Saver = () => Promise<boolean>;

export const UnsavedContext = createContext<UnsavedRegistry | null>(null);

type Claims = ReadonlyMap<
  string,
  Readonly<{ dirty: boolean; scope?: string; save?: Saver }>
>;

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
    (token: string, next: boolean, scope?: string, save?: Saver) => {
      parentClaim?.(token, next, scope, save);
      setClaims((held) => {
        const was = held.get(token);
        if (was?.dirty === next && was.scope === scope && was.save === save) {
          return held;
        }
        return new Map(held).set(token, { dirty: next, scope, save });
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

  return useMemo<UnsavedRegistry>(() => {
    const dirtyClaims = [...claims.values()].filter((entry) => entry.dirty);
    return {
      dirty: dirtyClaims.length > 0,
      claim,
      release,
      dirtyScopes: new Set(
        dirtyClaims.flatMap((entry) => (entry.scope ? [entry.scope] : [])),
      ),
      savers: dirtyClaims.every((entry) => entry.save)
        ? dirtyClaims.flatMap((entry) => (entry.save ? [entry.save] : []))
        : [],
    };
  }, [claims, claim, release]);
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
 *
 * `save` lets the leave question offer Save as well as Discard. It must be
 * stable across renders, or every render re-claims.
 */
export function useUnsavedGuard(
  dirty: boolean,
  scope?: string,
  save?: Saver,
): void {
  const { claim, release } = useContext(UnsavedContext) ?? {};
  // One token per mounting, so two rows of the same card are two claims.
  const token = useId();

  useEffect(() => {
    claim?.(token, dirty, scope, save);
  }, [claim, token, dirty, scope, save]);

  // A claim left after unmount would guard a draft that no longer exists.
  useEffect(() => () => release?.(token), [release, token]);
}
