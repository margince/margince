import { ArrowLeft, PanelLeftClose, PanelLeftOpen, Search } from "lucide-react";
import { type RefObject, Suspense, useEffect, useId, useRef } from "react";
import { AppLink } from "../design-system/applink";
import { Kbd } from "../design-system/atoms";
import { Breadcrumb, type Crumb } from "../design-system/breadcrumb";
import { SilentCardBoundary } from "../design-system/cardboundary";
import { IconAction } from "../design-system/iconaction";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import { useLocale, useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { EXTENSION_SCREEN, findExtension } from "../lib/extensions";
import {
  ariaKeyShortcuts,
  binding,
  shortcutCaps,
  shortcutTip,
} from "../lib/keymap";
import {
  leaveRecord,
  listAddress,
  rememberListAddress,
} from "../lib/listreturn";
import { useReadsTeamProgress } from "../lib/progressreader";
import { type Route, routeHash, useHash } from "../lib/router";
import { usePhoneViewport } from "../lib/viewport";
import { AccountMenu } from "./account";
import { useFiltersCrumbs } from "./filterstrail";
import {
  entryLabel,
  NAV,
  type NavSection,
  navLabelKey,
  recordKindOf,
  reservedPageTitle,
} from "./nav";
import { NotificationBell } from "./notificationbell";
import { OutboxButton } from "./outboxbutton";
import { resolveTitle, sectionHead, useRouteSubject } from "./pagemeta";
import { useHomeLinkLabel, WorkspaceChip } from "./shellrail";
import "./topbar.css";

const SetupReminder = lazy(
  async () => ({ default: (await import("./setup-reminder")).SetupReminder }),
  "essential",
  SilentCardBoundary,
);

// The top bar: the one strip that is true of the whole session rather than of
// the page under it — where you are (the trail), how you reach anything (the
// search), what is waiting for you (the bell), and who you are signed in as.
//
// It stands on the sidebar's own ground with a rule under it, so the chrome
// reads as one L-shaped frame around the content rather than as two panels that
// happen to touch. The page's own name is NOT here: it belongs to the document,
// scrolls with it, and stands in the content column above the content it names.
//
// Three tracks, the outer two equal, so the search sits at the middle of the
// CONTENT column — the reference this follows centres there too, and a field
// centred on the window sits visibly left of the column it stands in.

// A page that stands below a list: a record, or a page a record screen reserves
// a segment for. An id that names no record (a settings tab) is still the
// screen. The trail and the back control both lead to that list.
function belowList(route: Route): boolean {
  return Boolean(recordKindOf(route) && route.id) || !!reservedPageTitle(route);
}

/**
 * The trail for a route: where this page sits, ending in the page itself.
 *
 * A hook rather than a function because the last segment of a record's trail is
 * the record's NAME, which is a read — and a trail that printed a uuid until the
 * read landed would be a different sentence on every page open. `useEntityName`
 * and `useFiltersCrumbs` are called on every route so the hook order never
 * depends on which page is on screen; neither asks for a name nobody needs.
 */
function useCrumbs(route: Route, section?: NavSection): readonly Crumb[] {
  const t = useT();
  const { locale } = useLocale();
  const navItem = NAV.find((item) => item.screen === route.screen);
  const labelKey = navLabelKey(navItem, useReadsTeamProgress());
  const subject = useRouteSubject(route);
  const inSection = sectionHead(section, route);
  const unit =
    route.screen === EXTENSION_SCREEN ? findExtension(route.id) : null;
  const filtersCrumbs = useFiltersCrumbs(route);

  if (belowList(route)) {
    // A record's trail leads back to the list it was opened from. The list
    // keeps the filter the reader left it with.
    return [
      {
        label: resolveTitle(route.screen, labelKey, t),
        href: listAddress(route.screen),
      },
      { label: subject },
    ];
  }
  if (unit) {
    // A composed unit lives under Settings — it is offered from the page
    // holding the credential it is configured with, and has no index of its own.
    // Its leaf is the SUBJECT, like every other leaf here: the name the unit
    // carries is resolved once, in pagemeta, so the trail and the agent below it
    // cannot disagree about what this page is.
    return [
      {
        label: resolveTitle(SETTINGS_SCREEN, undefined, t),
        href: routeHash({ screen: SETTINGS_SCREEN }),
      },
      { label: subject },
    ];
  }
  if (inSection) {
    return [
      {
        label: t(inSection.section.titleKey),
        href: routeHash({ screen: route.screen }),
      },
      { label: entryLabel(inSection.entry, locale, t) },
    ];
  }
  if (filtersCrumbs) {
    return filtersCrumbs;
  }
  return [{ label: resolveTitle(route.screen, labelKey, t) }];
}

/**
 * The search affordance, centred in the bar at every width that can hold it.
 *
 * It is a BUTTON styled as a field and never accepts inline typing (AC-shell-7,
 * one search affordance): what it opens is the command palette, which is also
 * what ⌘K opens. Below the width where a centred field and the trail beside it
 * would collide it drops to its glyph — the shortcut is invisible to anyone who
 * does not already know it, and on touch there is no ⌘K at all, so the
 * affordance itself may never leave.
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

/** The trail, mounted only where it is drawn, so a phone reads no names for it. */
function TopBarTrail({
  route,
  section,
}: Readonly<{ route: Route; section?: NavSection }>) {
  const t = useT();
  const crumbs = useCrumbs(route, section);
  return <Breadcrumb items={crumbs} label={t("shell.breadcrumbAria")} />;
}

/** The company's mark leading home, where a phone has no room for the trail. */
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
  section,
  collapsed,
  onToggle,
  onOpenSearch,
}: Readonly<{
  route: Route;
  section?: NavSection;
  collapsed: boolean;
  onToggle?: () => void;
  onOpenSearch: () => void;
}>) {
  const t = useT();
  const hash = useHash();
  // Every list the bar stands over is noted, so a record opened from it can
  // lead back to it with the same filter.
  useEffect(() => rememberListAddress(hash), [hash]);
  const toggleLabel = collapsed ? t("shell.expand") : t("shell.collapse");
  const searchRef = useRef<HTMLButtonElement>(null);
  // The page heading under the bar already names the page, and the account
  // menu carries the setup reminder at this width.
  const phone = usePhoneViewport();
  return (
    <header className="topbar">
      <div className="topbar-lead">
        {onToggle && (
          <button
            type="button"
            className="topbar-toggle"
            aria-label={toggleLabel}
            // The shortcut belongs in the tooltip, not in the accessible name:
            // a speech-input user says the words they can read, and "Collapse
            // sidebar ⌘B" is not one of them.
            title={shortcutTip(
              toggleLabel,
              binding("sidebar").keys,
              t("shell.shortcuts.then"),
            )}
            aria-expanded={!collapsed}
            onClick={onToggle}
          >
            {collapsed ? (
              <PanelLeftOpen size={17} aria-hidden />
            ) : (
              <PanelLeftClose size={17} aria-hidden />
            )}
          </button>
        )}
        {/* The installed app has no browser Back, so a record page carries
            its own way out. */}
        {belowList(route) && (
          <IconAction
            label={t("shell.back")}
            icon={<ArrowLeft />}
            onClick={() => leaveRecord(route)}
          />
        )}
        {phone ? (
          <TopBarHome />
        ) : (
          <TopBarTrail route={route} section={section} />
        )}
      </div>
      <TopBarSearch onOpenSearch={onOpenSearch} buttonRef={searchRef} />
      <div className="topbar-trail">
        {/* Before the account chip, which is the strip's last word: the bell is
            something waiting FOR the reader, and the chip is who the reader is.
            Reading right to left, identity is the anchor and everything that
            wants their attention stacks up before it. */}
        {/* Keyed on the route: a step's action navigates, and the new page
            should not open under a panel left over from the last one. */}
        {!phone && (
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
