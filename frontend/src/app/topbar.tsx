import { ArrowLeft, Search } from "lucide-react";
import { type RefObject, Suspense, useEffect, useId, useRef } from "react";
import { vcardImportParent } from "../components/vcardimportlink";
import { AppLink } from "../design-system/applink";
import { Kbd } from "../design-system/atoms";
import { SilentCardBoundary } from "../design-system/cardboundary";
import { IconAction } from "../design-system/iconaction";
import { worklistReturnHref } from "../features/worklist/worklist.return";
import { useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { stepBackOutOf } from "../lib/historydepth";
import { useFirstScreenSettled } from "../lib/journeymarks";
import { ariaKeyShortcuts, binding, shortcutCaps } from "../lib/keymap";
import {
  isOnRecord,
  leaveRecord,
  leftRecord,
  rememberListAddress,
} from "../lib/listreturn";
import {
  navigateReplacing,
  type Route,
  routeHash,
  samePage,
  useHash,
} from "../lib/router";
import { usePhoneViewport } from "../lib/viewport";
import { AccountMenu } from "./account";
import { recordKindOf, reservedPageTitle, underSettings } from "./nav";
import { NotificationBell } from "./notificationbell";
import { ONBOARDING_ENTRY } from "./onboarding-gate";
import { OutboxButton } from "./outboxbutton";
import { useHomeLinkLabel, WorkspaceChip } from "./shellrail";
import "./topbar.css";

const SetupReminder = lazy(
  async () => ({ default: (await import("./setup-reminder")).SetupReminder }),
  "essential",
  SilentCardBoundary,
);

// The top bar: the one strip that is true of the whole session rather than of
// the page under it. The search leads; what is waiting and who is signed in
// stand at the end. The page's own name scrolls with the document it names.

// A page that stands below a list: a record, or a page a record screen reserves
// a segment for. An id that names no record (a settings tab) is still the
// screen. The back control leads to that list.
function belowList(route: Route): boolean {
  return Boolean(recordKindOf(route) && route.id) || !!reservedPageTitle(route);
}

// A bar still on the record leaves it. A bar that already left it as Back
// would is let finish; a pending move anywhere else, Back undoes.
function goBack(onScreen: Route, bar: string, shown: string): void {
  const parent = vcardImportParent(shown);
  if (isOnRecord(onScreen, bar)) {
    // Read off the address, not history, so a reload keeps the way back.
    const worklist = worklistReturnHref(bar);
    if (worklist) {
      globalThis.location.hash = worklist;
    } else {
      leaveRecord(onScreen, parent);
    }
    return;
  }
  if (leftRecord(onScreen, bar, parent)) return;
  stepBackOutOf(
    (hash) => samePage(hash, bar),
    () => navigateReplacing(onScreen),
  );
}

/**
 * The search affordance. A button styled as a field that opens the command
 * palette, as ⌘K does (AC-shell-7: one search affordance). Narrow, it drops to
 * its glyph and never leaves the bar: on touch there is no ⌘K.
 */
export function TopBarSearch({
  onOpenSearch,
  buttonRef,
}: Readonly<{
  onOpenSearch: () => void;
  buttonRef: RefObject<HTMLButtonElement | null>;
}>) {
  const t = useT();
  const label = t("shell.searchEverything");
  return (
    <div className="topbar-searchslot">
      <button
        ref={buttonRef}
        type="button"
        className="topbar-search"
        // Named by its printed words alone; the caps reach a screen reader as
        // the shortcut they are, never as part of the name.
        aria-keyshortcuts={ariaKeyShortcuts(binding("search").keys)}
        onClick={onOpenSearch}
      >
        <Search size={16} strokeWidth={1.8} aria-hidden />
        <span className="topbar-searchlabel">{label}</span>
        {/* One cap per KEY, which is how a keyboard shortcut is read: "⌘ then K",
            not the four-character string "⌘K" in one box. The caps come from the
            keymap the dispatcher reads, so the cap cannot name another key. */}
        <span className="topbar-keys" aria-hidden>
          {shortcutCaps("search", t).map((cap) => (
            <Kbd key={cap}>{cap}</Kbd>
          ))}
        </span>
      </button>
    </div>
  );
}

/** The company's mark leading home, on a phone where the rail is a bar. */
function TopBarHome() {
  const label = useHomeLinkLabel();
  const chipId = useId();
  const labelId = useId();
  // WCAG 2.5.3: the name starts with whatever the chip prints, read off the chip.
  return (
    <AppLink
      className="topbar-home"
      href="#/home"
      aria-labelledby={`${chipId} ${labelId}`}
    >
      <WorkspaceChip id={chipId} />
      <span id={labelId} hidden>
        {label}
      </span>
    </AppLink>
  );
}

export function TopBar({
  route,
  address,
  onOpenSearch,
  held = false,
}: Readonly<{
  route: Route;
  /** The page on screen's whole address, dials included; the bar's if absent. */
  address?: string;
  onOpenSearch: () => void;
  /** The onboarding gate holds the reader: a way back to setup stands where
   *  the setup reminder, the outbox and the bell would. */
  held?: boolean;
}>) {
  const t = useT();
  const hash = useHash();
  // Every list the bar stands over is noted, so a record opened from it can
  // lead back to it with the same filter.
  useEffect(() => rememberListAddress(hash), [hash]);
  const searchRef = useRef<HTMLButtonElement>(null);
  // The page heading under the bar already names the page, and the account
  // menu carries the setup reminder at this width.
  const phone = usePhoneViewport();
  const sideReadsMayStart = useFirstScreenSettled();
  const back = belowList(route);
  const toWorklist = back && worklistReturnHref(hash) !== undefined;
  return (
    <header className="topbar">
      {(back || phone) && (
        <div className="topbar-lead">
          {/* The installed app has no browser Back, so a record page carries
              its own way out. */}
          {back && (
            <IconAction
              label={t(toWorklist ? "brief.queue.back" : "shell.back")}
              icon={<ArrowLeft />}
              onClick={() => goBack(route, hash, address ?? hash)}
            />
          )}
          {phone && <TopBarHome />}
        </div>
      )}
      <TopBarSearch onOpenSearch={onOpenSearch} buttonRef={searchRef} />
      <div className="topbar-trail">
        {/* Before the account chip, which is the strip's last word: the bell is
            something waiting FOR the reader, and the chip is who the reader is.
            Reading right to left, identity is the anchor and everything that
            wants their attention stacks up before it. */}
        {/* Keyed on the route: a step's action navigates, and the new page
            should not open under a panel left over from the last one. */}
        {!phone && !held && sideReadsMayStart && (
          <Suspense fallback={null}>
            <SetupReminder
              key={routeHash(route)}
              onHide={() => searchRef.current?.focus()}
            />
          </Suspense>
        )}
        {held ? (
          <AppLink className="btn btn-ghost" href={routeHash(ONBOARDING_ENTRY)}>
            {t("shell.backToSetup")}
          </AppLink>
        ) : (
          <>
            <OutboxButton />
            <NotificationBell />
          </>
        )}
        {/* The phone shows no Settings level, so the menu that leads there
            says the page is one of its own. */}
        <AccountMenu current={phone && underSettings(route)} />
      </div>
    </header>
  );
}
