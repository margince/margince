import { UserRound } from "lucide-react";
import {
  type KeyboardEvent,
  lazy,
  type RefObject,
  Suspense,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import type { components } from "../api/schema";
import { AppLink } from "../design-system/applink";
import { Avatar } from "../design-system/atoms";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import { useT } from "../i18n";
import { routeHash } from "../lib/router";
import { useMe } from "../lib/session";
import { FlyoutRow, useFlyout } from "./accountflyout";
import { SignOutRow } from "./accountfoot";
import { HelpSubmenu } from "./accounthelp";
import { type RowSeat, useRovingSeats } from "./accountseats";
import { usePopoverDismiss } from "./popover";
import "./account.css";
import { ThemeSubmenu } from "./accounttheme";

// Lazy: the list reads the Brief's deck vocabulary, and most sessions never open it.
const ShortcutsDialog = lazy(() =>
  import("./shortcutsdialog").then((module) => ({
    default: module.ShortcutsDialog,
  })),
);

export { THEME_LABEL_KEYS } from "./accounttheme";

type SessionUser = components["schemas"]["MeResponse"]["user"];

/**
 * The product's ONE settings door.
 *
 * The sidebar's foot used to carry a second one beside this menu; it is gone,
 * and everything that reached the settings level through it reaches it here.
 * Derived from the router rather than spelled, so the door cannot come to point
 * at a hash the router does not parse.
 */
const SETTINGS_HREF = routeHash({ screen: SETTINGS_SCREEN });

/**
 * Where each row of the menu sits in the roving walk. A flyout row's seat is
 * read twice, by its tabstop and by the flyout that hands focus back to it.
 */
const SETTINGS_SEAT = 0;

const THEME_SEAT = 1;

const BOOKING_SEAT = 2;

const HELP_SEAT = 3;

const SIGN_OUT_SEAT = 4;

const SEAT_COUNT = 5;

/**
 * Who is signed in, in the shapes the block prints them in.
 *
 * Everything here is derived from what the session actually carries: a contact
 * without a display name is their address, and nobody is ever given a name the
 * product made up to fill the line.
 */
function identityOf(user: SessionUser | undefined) {
  const name = user?.display_name ?? "";
  const email = user?.email ?? "";
  const label = name || email;
  return {
    id: user?.id ?? "",
    name,
    email,
    label,
    // Both lines in one sentence, for the state that has no room to print them.
    spoken: name && email ? `${name} — ${email}` : label,
  };
}

type Identity = ReturnType<typeof identityOf>;

/**
 * The contact, printed: their display name when the record carries one, otherwise
 * the address alone — which is then not repeated underneath itself.
 */
function IdentityLines({ identity }: Readonly<{ identity: Identity }>) {
  return (
    <span className="acctwho">
      <b>{identity.label}</b>
      {identity.name && identity.email && (
        <span className="acctmail t-caption">{identity.email}</span>
      )}
    </span>
  );
}

/**
 * The way into settings, the only one the product has. `onActivate` closes the
 * menu, so it does not stay open over the page the link opens.
 */
function SettingsRow({
  seat,
  onActivate,
}: Readonly<{ seat: RowSeat; onActivate?: () => void }>) {
  const t = useT();
  return (
    <AppLink
      className="acctrow"
      href={SETTINGS_HREF}
      role="menuitem"
      {...seat}
      onClick={onActivate}
    >
      {t("nav.settings")}
    </AppLink>
  );
}

/**
 * A real `role="menu"`, so a keyboard reader can reach both submenus. The
 * identity block carries no role, since it is a statement. It also names the
 * menu, so a reader in menu mode hears whose account they act on.
 */
function AccountPanel({
  identity,
  id,
  panel,
  onDismiss,
  onShortcuts,
}: Readonly<{
  identity: Identity;
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  onDismiss: () => void;
  onShortcuts: () => void;
}>) {
  const t = useT();
  const { seat, walk, move } = useRovingSeats(SEAT_COUNT);
  const theme = useFlyout(THEME_SEAT, move);
  const help = useFlyout(HELP_SEAT, move);

  // The menu takes focus when it opens. A panel that leaves focus on the trigger
  // is one the keyboard reader has to walk into with a key nothing told them
  // about, and it is what makes the Escape return below mean anything.
  useEffect(() => {
    move(SETTINGS_SEAT);
  }, [move]);

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // Tab leaves the menu, so the menu goes with it (the APG menu-button
    // pattern). Not prevented: the reader asked to move on.
    if (event.key === "Tab") {
      onDismiss();
      return;
    }
    if (walk(event.key)) {
      event.preventDefault();
    }
  };

  return (
    <div
      className="accountmenu"
      id={id}
      role="menu"
      aria-label={
        identity.spoken
          ? `${t("shell.accountAria")} — ${identity.spoken}`
          : t("shell.accountAria")
      }
      // Focusable only by script: the roving tabstop lives on the rows.
      tabIndex={-1}
      ref={panel}
      onKeyDown={onKeyDown}
    >
      {/* Nothing at all rather than an empty line while /me is in flight or
          carries neither a name nor an address. */}
      {identity.label && (
        <>
          <div className="acctmenuwho">
            <IdentityLines identity={identity} />
          </div>
          <hr />
        </>
      )}
      <SettingsRow seat={seat(SETTINGS_SEAT)} onActivate={onDismiss} />
      <FlyoutRow
        label={t("shell.theme")}
        seat={seat(THEME_SEAT)}
        flyout={theme}
      >
        <ThemeSubmenu id={theme.id} panel={theme.panel} onBack={theme.close} />
      </FlyoutRow>
      {/* The groups: where you go, then the way out. An <hr> rather than a border
          on the last row — it separates a group, and a screen reader is told so. */}
      <hr />
      <AppLink
        className="acctrow"
        href={routeHash({ screen: "settings", id: "meetings" })}
        role="menuitem"
        {...seat(BOOKING_SEAT)}
        onClick={onDismiss}
      >
        {t("scheduling.myLink")}
      </AppLink>
      <FlyoutRow
        label={t("shell.helpMenu")}
        seat={seat(HELP_SEAT)}
        flyout={help}
        popup="menu"
      >
        <HelpSubmenu
          id={help.id}
          panel={help.panel}
          onBack={help.close}
          onLeave={onDismiss}
          onShortcuts={onShortcuts}
        />
      </FlyoutRow>
      <hr />
      <SignOutRow seat={seat(SIGN_OUT_SEAT)} />
    </div>
  );
}

/**
 * The account menu owns settings, appearance, the booking link, help and
 * feedback with the keyboard shortcuts, and sign-out.
 */
export function AccountMenu() {
  const t = useT();
  const me = useMe();
  const [open, setOpen] = useState(false);
  const [shortcuts, setShortcuts] = useState<"unopened" | "open" | "closed">(
    "unopened",
  );
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const spokenId = useId();
  const menuId = useId();

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
  const dismiss = useCallback(() => {
    const held = menu.current?.contains(document.activeElement) ?? false;
    setOpen(false);
    if (held) {
      trigger.current?.focus();
    }
  }, []);

  const identity = identityOf(me.data?.user);

  // One dismissal for every popover in the chrome (app/popover.ts): Escape from
  // anywhere inside, any outside click, and the opening click deferred past.
  usePopoverDismiss(open, menu, dismiss);

  return (
    <div className="account">
      <button
        type="button"
        className="user"
        ref={trigger}
        // Where the row PRINTS the contact's name, WCAG 2.5.3 (Label in Name)
        // requires that visible text to be part of the accessible name. This
        // trigger prints none — it is the avatar alone — so it is named by what
        // it does, and the contact it belongs to is carried as its DESCRIPTION
        // below.
        aria-label={t("shell.accountAria")}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        // With no room to print who is signed in, the sentence is carried for a
        // screen reader and clipped for the eye. It is wired as the DESCRIPTION
        // rather than left to be read in flow, because `aria-label` above
        // replaces the button's contents for name computation and would
        // otherwise silence it. The `title` is the pointer's version of the same
        // sentence, and never the accessible name.
        aria-describedby={identity.spoken ? spokenId : undefined}
        title={identity.spoken || undefined}
        onClick={() => setOpen((current) => !current)}
      >
        {identity.label ? (
          // The one chip, from the design system, keyed on the seat's user id
          // like every other chip drawn for this colleague.
          <Avatar identity={identity.id} name={identity.label} />
        ) : (
          // The session has not resolved yet. The chip keeps its box so the row
          // does not jump when the name arrives, and shows a contact glyph rather
          // than an empty circle or initials of a name nobody has.
          <span className="avatar avatar-sm" aria-hidden>
            <UserRound size={15} />
          </span>
        )}
        {identity.spoken && (
          <span className="sr-only" id={spokenId}>
            {identity.spoken}
          </span>
        )}
      </button>
      {open && (
        <AccountPanel
          identity={identity}
          id={menuId}
          panel={menu}
          onDismiss={dismiss}
          onShortcuts={() => {
            dismiss();
            setShortcuts("open");
          }}
        />
      )}
      {/* Mounted from the first opening on, so closing plays Modal's exit
          rather than pulling the dialog out from under it. */}
      {shortcuts !== "unopened" && (
        <Suspense fallback={null}>
          <ShortcutsDialog
            open={shortcuts === "open"}
            onClose={() => setShortcuts("closed")}
            returnFocusTo={() => trigger.current}
          />
        </Suspense>
      )}
    </div>
  );
}
