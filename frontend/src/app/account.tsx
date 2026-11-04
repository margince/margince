import { UserRound } from "lucide-react";
import {
  type KeyboardEvent,
  lazy,
  type ReactNode,
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
import { Avatar, Button } from "../design-system/atoms";
import { StatusBadge } from "../design-system/statusbadge";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import { useT } from "../i18n";
import { routeHash } from "../lib/router";
import {
  bookingPagePaused,
  useSchedulingProfile,
} from "../lib/scheduling-profile-query";
import { useMe } from "../lib/session";
import type { SetupChecklist } from "../lib/setup-checklist";
import { usePhoneViewport } from "../lib/viewport";
import { FlyoutRow, useFlyout } from "./accountflyout";
import { SignOutRow } from "./accountfoot";
import { HelpSubmenu } from "./accounthelp";
import { type RowSeat, useRovingSeats } from "./accountseats";
import { usePopoverDismiss } from "./popover";
import "./account.css";
import "./topbar-popover.css";
import { ThemeSubmenu } from "./accounttheme";
import {
  nextStepRoute,
  SetupReminderLabel,
  useSetupReminder,
} from "./setup-reminder";

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

/** The seat's own page, which the menu's head opens. */
const ACCOUNT_HREF = routeHash({ screen: SETTINGS_SCREEN, id: "account" });

/**
 * Where each row of the menu sits in the roving walk. A flyout row's seat is
 * read twice, by its tabstop and by the flyout that hands focus back to it.
 */
const WHO_SEAT = 0;

const SETTINGS_SEAT = 1;

const THEME_SEAT = 2;

const BOOKING_SEAT = 3;

const HELP_SEAT = 4;

const SETUP_SEAT = 5;

// Sign out keeps its seat while the setup row is absent, which the walk skips.
// So a row arriving with the menu open moves neither focus nor the tabstop.
const SIGN_OUT_SEAT = 6;

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
 * A row that opens a page in the app. `onActivate` closes the menu, so it does
 * not stay open over the page the link opens.
 */
function LinkRow({
  href,
  seat,
  onActivate,
  className,
  label,
  children,
}: Readonly<{
  href: string;
  seat: RowSeat;
  onActivate: () => void;
  className?: string;
  label?: string;
  children: ReactNode;
}>) {
  return (
    <AppLink
      className={className ? `acctrow ${className}` : "acctrow"}
      href={href}
      role="menuitem"
      aria-label={label}
      {...seat}
      onClick={onActivate}
    >
      {children}
    </AppLink>
  );
}

/**
 * A real `role="menu"`, so a keyboard reader can reach both submenus. Its head
 * is the identity, a row into the seat's own account page. Focus lands on it,
 * so a reader in menu mode hears whose account they act on.
 */
function AccountPanel({
  identity,
  id,
  panel,
  setup,
  onDismiss,
  onFocusLost,
  onShortcuts,
}: Readonly<{
  identity: Identity;
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  setup: SetupChecklist | null;
  onDismiss: () => void;
  onFocusLost: () => void;
  onShortcuts: () => void;
}>) {
  const t = useT();
  const { seat, walk, move, holds } = useRovingSeats(SIGN_OUT_SEAT + 1);
  const theme = useFlyout(THEME_SEAT, move);
  const help = useFlyout(HELP_SEAT, move);
  // A paused booking link is dark to every guest who holds it, so the menu
  // that opens it says so.
  const booking = useSchedulingProfile().data;

  // The menu takes focus when it opens. A panel that leaves focus on the trigger
  // is one the keyboard reader has to walk into with a key nothing told them
  // about, and it is what makes the Escape return below mean anything.
  const hasHead = Boolean(identity.label);
  const headless = useRef(!hasHead);
  useEffect(() => {
    move(headless.current ? SETTINGS_SEAT : WHO_SEAT);
  }, [move]);
  // A head that arrives late takes focus only from the row focus first landed
  // on, never from a row the reader walked to.
  useEffect(() => {
    if (!hasHead || !headless.current) {
      return;
    }
    headless.current = false;
    if (holds(SETTINGS_SEAT)) {
      move(WHO_SEAT);
    }
  }, [hasHead, holds, move]);
  // The setup row unmounts when another tab hides the reminder. If it held
  // focus, focus is on <body> now and the arrows walk nothing, so the menu goes.
  const hadSetup = useRef(setup !== null);
  useEffect(() => {
    const dropped = hadSetup.current && setup === null;
    hadSetup.current = setup !== null;
    const fell = document.activeElement ?? document.body;
    if (dropped && fell === document.body) {
      onFocusLost();
    }
  }, [setup, onFocusLost]);

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
      className="accountmenu topbar-popover"
      id={id}
      role="menu"
      aria-label={t("shell.accountAria")}
      // Focusable only by script: the roving tabstop lives on the rows.
      tabIndex={-1}
      ref={panel}
      onKeyDown={onKeyDown}
    >
      {/* Nothing at all rather than an empty line while /me is in flight or
          carries neither a name nor an address. */}
      {hasHead && (
        <>
          <LinkRow
            href={ACCOUNT_HREF}
            seat={seat(WHO_SEAT)}
            onActivate={onDismiss}
            className="acctmenuwho"
            // The printed name and address lead, so a voice user can say them.
            label={t("shell.accountSettingsAria", { who: identity.spoken })}
          >
            {/* The one chip, keyed on the seat's user id like every other chip
                drawn for this colleague. */}
            <Avatar identity={identity.id} name={identity.label} />
            <IdentityLines identity={identity} />
          </LinkRow>
          <hr />
        </>
      )}
      <LinkRow
        href={SETTINGS_HREF}
        seat={seat(SETTINGS_SEAT)}
        onActivate={onDismiss}
      >
        {t("nav.settings")}
      </LinkRow>
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
      <LinkRow
        href={routeHash({ screen: "settings", id: "meetings" })}
        seat={seat(BOOKING_SEAT)}
        onActivate={onDismiss}
      >
        {t("scheduling.myLink")}
        {booking && bookingPagePaused(booking) && (
          <StatusBadge kind="attention" label={t("scheduling.status.paused")} />
        )}
      </LinkRow>
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
      {setup && (
        <LinkRow
          href={routeHash(nextStepRoute(setup))}
          seat={seat(SETUP_SEAT)}
          onActivate={onDismiss}
        >
          <SetupReminderLabel setup={setup} />
        </LinkRow>
      )}
      <hr />
      <SignOutRow seat={seat(SIGN_OUT_SEAT)} />
    </div>
  );
}

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
  // The top bar has no room for the setup reminder at phone width.
  const setup = useSetupReminder(usePhoneViewport());

  // One dismissal for every popover in the chrome (app/popover.ts): Escape from
  // anywhere inside, any outside click, and the opening click deferred past.
  usePopoverDismiss(open, menu, dismiss);

  return (
    <div className="account">
      {/* The bare glyph button its neighbours are, so every state answers alike. */}
      <Button
        variant="bare"
        iconOnly
        ref={trigger}
        // WCAG 2.5.3 (Label in Name) binds only printed text, and this glyph
        // prints none. So it is named by what it does, and the contact is its
        // description below.
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
        <UserRound aria-hidden />
        {identity.spoken && (
          <span className="sr-only" id={spokenId}>
            {identity.spoken}
          </span>
        )}
      </Button>
      {open && (
        <AccountPanel
          identity={identity}
          id={menuId}
          panel={menu}
          setup={setup}
          onDismiss={dismiss}
          onFocusLost={closeFromLostFocus}
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
