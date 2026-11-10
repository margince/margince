import { UserRound } from "lucide-react";
import { Suspense, useCallback, useId, useRef, useState } from "react";
import { Avatar, Button } from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { useMe } from "../lib/session";
import { useShortcutHandler } from "../lib/shortcuts";
import { identityOf } from "./accountidentity";
import { usePopoverDismiss } from "./popover";
import "./account.css";
import "./topbar-popover.css";

const AccountPanel = lazy(
  async () => ({
    default: (await import("./accountpanel")).AccountPanel,
  }),
  "complete",
  CardBoundary,
);

// Lazy: the list reads the Brief's deck vocabulary, and most sessions never open it.
const ShortcutsDialog = lazy(
  async () => ({
    default: (await import("./shortcutsdialog")).ShortcutsDialog,
  }),
  "complete",
  CardBoundary,
);

/**
 * The account menu owns settings, appearance, the booking link, help and
 * feedback with the keyboard shortcuts, and sign-out. At phone width it also
 * carries the setup reminder.
 */
export function AccountMenu() {
  const t = useT();
  const me = useMe();
  const [open, setOpen] = useState(false);
  const [shortcuts, setShortcuts] = useState<"unopened" | "open" | "closed">(
    "unopened",
  );
  // From Help, focus returns to the trigger; from `?`, to wherever the key was pressed.
  const [fromHelp, setFromHelp] = useState(false);
  useShortcutHandler("shortcuts", () => {
    setFromHelp(false);
    setShortcuts("open");
  });
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const spokenId = useId();
  const menuId = useId();
  const faceId = useId();
  const nameId = useId();

  /**
   * Close, and put focus back where it can be used.
   *
   * Dismissing the menu unmounts whatever row was focused, and an unmounted
   * focus owner leaves the document focused on `<body>` — from there a keyboard
   * user's next Tab starts at the top of the page, having lost the chrome they
   * were standing in. The account trigger is where they came from, so it is
   * where they go back to.
   *
   * Only when the menu actually HELD focus, which is the difference between
   * restoring focus and stealing it: an outside click usually lands on something
   * focusable of its own, and pulling focus onto the trigger after it would undo
   * what the click just did.
   */
  const close = useCallback((held: boolean) => {
    setOpen(false);
    if (held) {
      trigger.current?.focus();
    }
  }, []);
  const dismiss = useCallback(
    () => close(menu.current?.contains(document.activeElement) ?? false),
    [close],
  );
  const closeFromLostFocus = useCallback(() => close(true), [close]);

  const identity = identityOf(me.data?.user);

  // One dismissal for every popover in the chrome (app/popover.ts): Escape from
  // anywhere inside, any outside click, and the opening click deferred past.
  usePopoverDismiss(open, menu, dismiss);

  return (
    <div className="account">
      {/* The bare glyph button its neighbours are, so every state answers alike. */}
      <Button
        variant="bare"
        iconOnly
        className="account-trigger"
        ref={trigger}
        // WCAG 2.5.3 (Label in Name): the name starts with whatever the chip
        // prints, read off the chip itself, so a painted photo adds nothing.
        aria-labelledby={identity.label ? `${faceId} ${nameId}` : nameId}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        // Who is signed in, for a screen reader: a description, because the name
        // above leaves it out. The `title` is the pointer's copy, never the name.
        aria-describedby={identity.spoken ? spokenId : undefined}
        title={identity.spoken || undefined}
        onClick={() => setOpen((current) => !current)}
      >
        {identity.label ? (
          <span className="account-face" id={faceId} aria-hidden="true">
            <Avatar
              identity={identity.id}
              name={identity.label}
              photo={identity.photo}
            />
          </span>
        ) : (
          <UserRound aria-hidden />
        )}
        <span id={nameId} hidden>
          {t("shell.accountAria")}
        </span>
        {identity.spoken && (
          <span className="sr-only" id={spokenId}>
            {identity.spoken}
          </span>
        )}
      </Button>
      {open && (
        <Suspense fallback={null}>
          <AccountPanel
            identity={identity}
            id={menuId}
            panel={menu}
            onDismiss={dismiss}
            onFocusLost={closeFromLostFocus}
            onShortcuts={() => {
              dismiss();
              setFromHelp(true);
              setShortcuts("open");
            }}
          />
        </Suspense>
      )}
      {/* Mounted from the first opening on, so closing plays Modal's exit
          rather than pulling the dialog out from under it. */}
      {shortcuts !== "unopened" && (
        <Suspense fallback={null}>
          <ShortcutsDialog
            open={shortcuts === "open"}
            onClose={() => setShortcuts("closed")}
            returnFocusTo={() => (fromHelp ? trigger.current : null)}
          />
        </Suspense>
      )}
    </div>
  );
}
