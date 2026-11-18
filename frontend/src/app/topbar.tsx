import { ArrowLeft, Search } from "lucide-react";
import { type RefObject, Suspense, useEffect, useId, useRef } from "react";
import { AppLink } from "../design-system/applink";
import { Kbd } from "../design-system/atoms";
import { SilentCardBoundary } from "../design-system/cardboundary";
import { IconAction } from "../design-system/iconaction";
import { useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { useFirstScreenSettled } from "../lib/journeymarks";
import { ariaKeyShortcuts, binding, shortcutCaps } from "../lib/keymap";
import { leaveRecord, rememberListAddress } from "../lib/listreturn";
import { type Route, routeHash, useHash } from "../lib/router";
import { usePhoneViewport } from "../lib/viewport";
import { AccountMenu } from "./account";
import { recordKindOf, reservedPageTitle } from "./nav";
import { NotificationBell } from "./notificationbell";
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
          {shortcutCaps("search").map((cap) => (
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
  onOpenSearch,
}: Readonly<{
  route: Route;
  onOpenSearch: () => void;
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
  return (
    <header className="topbar">
      {(back || phone) && (
        <div className="topbar-lead">
          {/* The installed app has no browser Back, so a record page carries
              its own way out. */}
          {back && (
            <IconAction
              label={t("shell.back")}
              icon={<ArrowLeft />}
              onClick={() => leaveRecord(route)}
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
        {!phone && sideReadsMayStart && (
          <Suspense fallback={null}>
            <SetupReminder
              key={routeHash(route)}
              onHide={() => searchRef.current?.focus()}
            />
          </Suspense>
        )}
        <OutboxButton />
        <NotificationBell />
        <AccountMenu />
      </div>
    </header>
  );
}
