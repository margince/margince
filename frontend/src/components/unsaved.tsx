import { type ReactNode, useCallback, useEffect, useId, useState } from "react";
import { Button } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useT } from "../i18n";
import { navigate, type Route, useHash } from "../lib/router";
import {
  type Saver,
  UnsavedContext,
  useHasUnsavedChanges,
  useUnsavedGuard,
  useUnsavedRegistry,
} from "../lib/unsavedclaims";
import type { UrlParams } from "../lib/urlstate";

/**
 * Unsaved edits, and what happens when the reader leaves without saving them.
 *
 * A draft that is discarded without a word — a rewritten signature, a retyped
 * installation name, a voice profile somebody spent ten minutes on — is one
 * sidebar click away on every form in the product. Silently is the part that
 * matters: a form that warns you is annoying, a form that loses your work
 * without asking teaches you not to trust it with anything long.
 *
 * Two mechanisms, because there are two ways to leave and only one of them is
 * ours to intercept.
 *
 * A RELOAD or a closed tab is the browser's to ask about, through
 * `beforeunload`, and the browser's own dialog is the only thing allowed to
 * appear there — a page cannot render its own confirm at that point, and every
 * attempt to reads as a page fighting the window.
 *
 * A move WITHIN the app is ours, and it is asked with this product's own
 * `ConfirmModal`. `UnsavedGuard` is rendered ABOVE the routed screen (App.tsx),
 * because a guard can only hold the moves it is still mounted for: inside a
 * screen it caught that screen's own tabs and lost everything the instant the
 * reader left for another page. What it holds is CONTENT, never the URL. A guard
 * wired into `hashchange` would also intercept Back and Forward, where undoing
 * the browser's own history gesture means rewriting entries underneath the
 * reader — and the failure mode of getting that wrong is broken navigation for
 * everybody, far worse than the loss it prevents. So the subtree waits, the URL
 * is allowed to move, and the browser's gestures are left alone.
 */

export { type Saver, useHasUnsavedChanges, useUnsavedGuard };

/**
 * Guard `dirty` like `useUnsavedGuard`, and return the one way to leave
 * without being asked: after a save, or after a question the page already put.
 *
 * The guard learns of a release through a state update, and a navigation sent
 * beside it races that update to the guard. So leaving disarms this claim and
 * moves only once the guard reads it clear. Its own scope, so a draft held
 * elsewhere on the page still asks rather than holding the move forever.
 * Outside a provider nothing is held, and it moves on the next effect.
 */
export function useGuardedLeave(
  dirty: boolean,
): (route: Route, dials?: UrlParams) => void {
  const [leaving, setLeaving] = useState<Leaving | null>(null);
  const scope = useId();
  useUnsavedGuard(dirty && leaving === null, scope);
  const held = useHasUnsavedChanges(scope);
  const hash = useHash();

  // The other draft's question can be answered Keep, which puts the address
  // back with this page still mounted: its own draft is unsaved again.
  if (leaving?.sentTo !== undefined) {
    if (!leaving.arrived && hash === leaving.sentTo) {
      setLeaving({ ...leaving, arrived: true });
    } else if (leaving.arrived && hash !== leaving.sentTo) {
      setLeaving(null);
    }
  }

  useEffect(() => {
    if (leaving && leaving.sentTo === undefined && !held) {
      navigate(leaving.route, leaving.dials);
      setLeaving({ ...leaving, sentTo: globalThis.location.hash });
    }
  }, [leaving, held]);

  return useCallback(
    (route: Route, dials?: UrlParams) => setLeaving({ route, dials }),
    [],
  );
}

type Leaving = Readonly<{
  route: Route;
  dials?: UrlParams;
  /** The address the move wrote, once it has. */
  sentTo?: string;
  /** Whether that address has been read back since. */
  arrived?: boolean;
}>;

/**
 * The window's own question, asked only while something is actually unsaved.
 *
 * Registered and unregistered with the flag rather than installed once and
 * consulted, because a listener that is merely present is enough to make some
 * browsers treat the page as unload-blocking, which costs it the back/forward
 * cache. The text is the browser's — every engine has ignored a custom string
 * for a decade — so `preventDefault` is the whole of the API worth using.
 */
function useBeforeUnload(dirty: boolean): void {
  useEffect(() => {
    if (!dirty) {
      return;
    }
    const ask = (event: BeforeUnloadEvent) => event.preventDefault();
    globalThis.addEventListener("beforeunload", ask);
    return () => globalThis.removeEventListener("beforeunload", ask);
  }, [dirty]);
}

/**
 * Holds `children` steady while an unsaved edit is on screen and the address
 * changes underneath it.
 *
 * `address` is whatever the caller counts as "somewhere else" — in this app the
 * hash of the displayed route. When it changes and something is dirty, the guard
 * keeps rendering the address it was already showing and asks. Discarding shows
 * the new one; keeping sends the reader back to the one they were editing, which
 * is the only outcome that leaves their work where they can still save it.
 *
 * `onKeep` is how the caller puts the address back, because only the caller
 * knows what the address IS — this component holds content, not routes.
 */
export function UnsavedGuard<Address extends string>({
  address,
  onKeep,
  children,
}: Readonly<{
  address: Address;
  onKeep: (address: Address) => void;
  children: (address: Address) => ReactNode;
}>) {
  const registry = useUnsavedRegistry(null);
  // The address whose content is on screen. It follows `address` freely until a
  // draft is at stake, and then stops until the question is answered.
  const [shown, setShown] = useState(address);

  const dirty = registry.dirty;
  useBeforeUnload(dirty);

  // Catching up is not a state change, so it happens during render rather than
  // in an effect: an effect would paint the OLD content for one frame against
  // the NEW page heading the shell has already swapped, which reads as the page
  // half-navigating.
  if (address !== shown && !dirty) {
    setShown(address);
  }

  const asking = address !== shown && dirty;

  return (
    <UnsavedContext.Provider value={registry}>
      {children(shown)}
      <LeaveQuestion
        open={asking}
        savers={registry.savers}
        onStay={() => onKeep(shown)}
        onLeave={() => setShown(address)}
      />
    </UnsavedContext.Provider>
  );
}

/**
 * The question asked on the way out of a draft. Closing it is the safe answer,
 * so Escape and the backdrop both keep the edit. A dialog whose dismissal
 * destroys work punishes the reader for not reading it. With `savers` it
 * offers Save, and leaves only once every one of them has written.
 */
function LeaveQuestion({
  open,
  savers,
  onStay,
  onLeave,
}: Readonly<{
  open: boolean;
  savers: readonly Saver[];
  onStay: () => void;
  onLeave: () => void;
}>) {
  const t = useT();
  const [saving, setSaving] = useState(false);
  const [refused, setRefused] = useState(false);
  if (savers.length === 0) {
    return (
      <ConfirmModal
        open={open}
        title={t("unsaved.title")}
        confirmLabel={t("unsaved.discard")}
        confirmVariant="danger"
        onClose={onStay}
        onConfirm={onLeave}
      >
        <p>{t("unsaved.body")}</p>
      </ConfirmModal>
    );
  }
  // A refusal belongs to this asking; the next one starts clean.
  const answer = (then: () => void) => () => {
    setRefused(false);
    then();
  };
  const save = async () => {
    setSaving(true);
    setRefused(false);
    const saved = await Promise.all(savers.map((saver) => saver()));
    setSaving(false);
    if (saved.every(Boolean)) onLeave();
    else setRefused(true);
  };
  return (
    <ConfirmModal
      open={open}
      title={t("unsaved.saveTitle")}
      confirmLabel={t("saveBar.save")}
      cancelLabel={t("unsaved.stay")}
      pending={saving}
      error={refused ? t("unsaved.saveRefused") : null}
      actionsLead={
        <Button variant="ghost" disabled={saving} onClick={answer(onLeave)}>
          {t("saveBar.discard")}
        </Button>
      }
      onClose={answer(onStay)}
      onConfirm={() => {
        void save();
      }}
    >
      <p>{t("unsaved.saveBody")}</p>
    </ConfirmModal>
  );
}
