import {
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
  useEffect,
  useRef,
} from "react";
import { AppLink } from "../design-system/applink";
import { Avatar } from "../design-system/atoms";
import { StatusBadge } from "../design-system/statusbadge";
import { useT } from "../i18n";
import { routeHash } from "../lib/router";
import {
  bookingPagePaused,
  useSchedulingProfile,
} from "../lib/scheduling-profile-query";
import { SETTINGS_SCREEN } from "../lib/settingsrouting";
import { usePhoneViewport } from "../lib/viewport";
import { FlyoutRow, useFlyout } from "./accountflyout";
import { SignOutRow } from "./accountfoot";
import { HelpSubmenu } from "./accounthelp";
import type { Identity } from "./accountidentity";
import { type RowSeat, useRovingSeats } from "./accountseats";
import { ThemeSubmenu } from "./accounttheme";
import {
  nextStepRoute,
  SetupReminderLabel,
  useSetupReminder,
} from "./setup-reminder";

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
export function AccountPanel({
  identity,
  id,
  panel,
  onDismiss,
  onFocusLost,
  onShortcuts,
}: Readonly<{
  identity: Identity;
  id: string;
  panel: RefObject<HTMLDivElement | null>;
  onDismiss: () => void;
  onFocusLost: () => void;
  onShortcuts: () => void;
}>) {
  const t = useT();
  const setup = useSetupReminder(usePhoneViewport());
  const { seat, walk, move, holds } = useRovingSeats(SIGN_OUT_SEAT + 1);
  const theme = useFlyout(THEME_SEAT, move);
  const help = useFlyout(HELP_SEAT, move);
  // A paused booking link is dark to every guest who holds it, so the menu
  // that opens it says so.
  const booking = useSchedulingProfile().data;

  // Opening the menu must place the keyboard reader on its first row.
  const hasHead = Boolean(identity.label);
  const headless = useRef(!hasHead);
  useEffect(() => {
    move(headless.current ? SETTINGS_SEAT : WHO_SEAT);
  }, [move]);
  // A head that arrives late takes focus only from the row focus first landed
  // on. Once the reader has walked, even back to that row, focus stays put.
  const walked = useRef(false);
  useEffect(() => {
    if (!hasHead || !headless.current) {
      return;
    }
    headless.current = false;
    if (!walked.current && holds(SETTINGS_SEAT)) {
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
      walked.current = true;
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
            <Avatar
              identity={identity.id}
              name={identity.label}
              photo={identity.photo}
            />
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
