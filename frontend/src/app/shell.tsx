import { useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Menu, X } from "lucide-react";
import {
  type KeyboardEvent,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import type { components } from "../api/schema";
import { Avatar, Button, Modal } from "../design-system/atoms";
import { Logomark } from "../design-system/logomark";
import { useLocale, useT } from "../i18n";
import { SETTINGS_SCREEN, useSettingsSection } from "../screens/settings";
import { AgentEdge } from "./agent-edge";
import { AgentRail } from "./agentrail";
import { EconomyBanner } from "./economybanner";
import { EmbedReindexBanner } from "./embedreindexbanner";
import { SCREEN_ENTITY } from "./entity";
import { EXTENSION_SCREEN, findExtension } from "./extensions";
import {
  entryLabel,
  GRIDDED_RECORD_SCREENS,
  GRIDDED_SCREENS,
  MOBILE_PRIMARY,
  NAV,
  type NavCounts,
  type NavLevelEntry,
  type NavLevelGroup,
  type NavSection,
  navEntryHref,
  RAIL_LESS_SCREENS,
} from "./nav";
import {
  NavLevelView,
  NavWalkProvider,
  useNavLevel,
  useNavWalk,
} from "./navlevel";
import { PageAsideProvider, PageAsideRegion } from "./pageaside";
import { PAGE_SUB_KEYS, resolveTitle, sectionHead } from "./pagemeta";
import { usePopoverDismiss } from "./popover";
import { type Route, routeHash, useRoute } from "./router";
import { useScrollMemory } from "./scrollmemory";
import { TopBar } from "./topbar";
import { usePhoneViewport } from "./viewport";
import "./shell.css";

type CompanyProfile = components["schemas"]["CompanyProfile"];

// The app shell: one sidebar against the left edge of the viewport, and the
// content beside it. Collapsed the sidebar is the canonical 64px rail WDS-NAV-1
// specifies, unchanged — the labeled state is additive, so the spec stays true
// at 64px rather than being replaced.
//
// The chrome is an L: the sidebar carries where you can GO, and the top bar
// above the content (app/topbar.tsx) carries everything else that is true of the
// whole session — where you are, how you search, and who you are signed in as.
// The sidebar holds destinations and nothing else, because a panel that also
// held the search, the settings door and the person had four different kinds of
// row in one column and read as a list of everything.
//
// The content column carries only what is true of THIS screen: its heading,
// which stands INSIDE the scroller and scrolls with the page because it belongs
// to the document, and the agent, which floats at the column's foot.

// The attention counts the rail badges on the rows a level declares badgeable.
// They are the levels' own currency (app/subnav.ts), named here for the shell
// because this is the seam a caller hands them in at. No caller does today: the
// primary level badges nothing (app/nav.ts BADGE_SCREENS) now that the queues
// that had counts are lanes inside Today, which reports its numbers on the page.
// The prop stays because a deeper level declaring `badgeIds` needs this door.
export type ShellCounts = NavCounts;

const COLLAPSE_KEY = "margince.sidebarCollapsed";

// Storage is unavailable in some embedded contexts; a missing preference is a
// default, never an error.
function readStored(key: string): string | null {
  try {
    return window.localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeStored(key: string, value: string): void {
  try {
    window.localStorage.setItem(key, value);
  } catch {
    // A browser refusing storage must not break navigation.
  }
}

function BrandBlock() {
  const t = useT();
  // The installation's own organization (A107/ADR-0061: one installation, one
  // organization). Read from the cache the onboarding gate already filled
  // rather than observing ["company"] again: a second observer on that entry
  // re-triggers the gate's fetch and walks the app back through its splash.
  // The gate guarantees the profile is present before the shell mounts.
  const installation =
    useQueryClient().getQueryData<CompanyProfile | null>(["company"]) ??
    undefined;
  // Whose product this is, above whose product it runs on. The reader works
  // for the company named here and not for us, so the company is the heading
  // and the product is the line under it.
  //
  // Absent profile — the shell mounted before onboarding described anybody —
  // the block falls back to the product's own mark and name. A company name is
  // never invented to fill the line.
  if (!installation) {
    return (
      <a className="ws" href="#/home" aria-label={t("shell.logoAria")}>
        <span className="ws-chip">
          <Logomark />
        </span>
        <span className="ws-name">
          <b>{t("shell.logoAria")}</b>
        </span>
      </a>
    );
  }
  return (
    <a
      className="ws"
      href="#/home"
      aria-label={t("shell.companyLogoAria", {
        company: installation.display_name,
      })}
    >
      {/* The mark the onboarding website read resolved from the company's own
          site, in the slot the product's mark holds otherwise — one slot, so the
          brand block does not move when onboarding finishes and so the rail's
          own rules about the mark have one thing to name. Avatar draws its
          deterministic monogram underneath, and a company whose site declared no
          icon has a face rather than a gap. */}
      <span className="ws-chip ws-chip-company">
        <Avatar
          identity={installation.organization_id}
          name={installation.display_name}
          src={installation.logo_url}
          shape="organization"
        />
      </span>
      <span className="ws-name">
        <b>{installation.display_name}</b>
        <span className="ws-org">{t("shell.poweredBy")}</span>
      </span>
    </a>
  );
}

// WCAG 2.4.1, Bypass Blocks. Every page in the product puts the same block ahead
// of its content — up to twelve navigation rows and the entitlement row in the
// sidebar, then the strip's collapse control, trail, search, system-of-record
// chip, approvals bell and account menu — and without this a keyboard reader
// walked all of it again on every page they opened.
//
// The destination is the SCROLLER, not the content column: the strip is the first
// row of that column, so a skip that landed on `.main` put the reader in front of
// the very chrome they asked to skip.
//
// A button, not the conventional `<a href="#content">`, because this app is
// hash-routed: setting the hash to "#content" is a NAVIGATION here, and the
// router would parse it as a screen name and land on the unknown-page fallback.
// Fragment links and hash routing cannot both own the hash, so the affordance
// keeps the behaviour (move focus to the content) and gives up the spelling.
//
// It sits first in the DOM so it is the first thing Tab reaches, and stays out of
// sight until it has focus (shell.css).
function SkipToContent({
  target,
}: Readonly<{ target: RefObject<HTMLElement | null> }>) {
  const t = useT();
  return (
    <button
      type="button"
      className="skiplink"
      onClick={() => target.current?.focus()}
    >
      {t("shell.skipToContent")}
    </button>
  );
}

// The panel's own state as classes: the width it is at, whether the phone sheet
// is open, whether it is mid-collapse, and whether it is showing a drilled-in
// LEVEL. That last one is a class rather than a CSS `:has()` on the level's own
// markup, whose specificity is its argument's and would have outranked the
// sheet's layout from a rule that only means to arrange a panel.
function railClasses(
  state: Readonly<{
    collapsed: boolean;
    sheetOpen: boolean;
    leveled: boolean;
  }>,
): string {
  return [
    "rail",
    state.collapsed ? "collapsed" : "expanded",
    state.sheetOpen ? "sheetopen" : "",
    state.leveled ? "leveled" : "",
  ]
    .filter((name) => name !== "")
    .join(" ");
}

type RailProps = {
  route: Route;
  counts?: ShellCounts;
  collapsed?: boolean;
};

export function WorkspaceRail({
  route,
  counts,
  collapsed = false,
  section,
}: Readonly<RailProps & { section?: NavSection }>) {
  const t = useT();
  // Collapsed items are icon-only, so the label needs a tooltip that satisfies
  // WCAG 1.4.13: it appears on keyboard focus as well as hover, stays visible
  // while the pointer is on it (it renders inside the hovered wrapper), and is
  // dismissible with Escape without moving focus. The tooltip is never the
  // accessible name — aria-label carries that in both states.
  const [tip, setTip] = useState<string | null>(null);
  // At phone width the same markup is a bottom bar; More expands it into a
  // sheet carrying every destination. One nav element, so there is still exactly
  // one navigation landmark and no second item list to keep in sync.
  const [sheetOpen, setSheetOpen] = useState(false);
  const phone = usePhoneViewport();
  const nav = useRef<HTMLElement>(null);
  const more = useRef<HTMLButtonElement>(null);
  const dismissTip = useCallback((event: KeyboardEvent) => {
    if (event.key === "Escape") {
      setTip(null);
    }
  }, []);

  // The sheet is a popover over the page, so it dismisses like every other one
  // in the chrome: Escape from anywhere, any click outside it — which is what
  // makes the scrim behind it work without being a control of its own.
  const closeSheet = useCallback(() => setSheetOpen(false), []);
  usePopoverDismiss(sheetOpen, nav, closeSheet);

  // Which of the route's levels the panel is showing, and the two ways the
  // reader moves between them (app/navlevel.tsx). A section's entries take the
  // panel OVER rather than hanging off the destinations: 64px cannot carry two
  // levels, and 252px carrying both reads as a list of twenty places to go.
  //
  // At phone width the panel is a bottom bar of four destinations, and it KEEPS
  // them on a section route — a bar that hands its four tabs over to a section
  // loses every destination and is left holding two controls. The section is
  // reached from the page head there instead (SectionSwitcher below), so no
  // section is walked into here.
  const level = useNavLevel(
    route,
    phone ? undefined : section,
    nav,
    closeSheet,
  );

  // Opening a sheet that covers the page has to take focus with it, or a
  // keyboard user is left tabbing through a page they can no longer see; and
  // closing it has to hand focus back to the control that opened it rather than
  // dropping it on <body>. Only when the sheet still HOLDS focus — a row that
  // navigated away has already put focus somewhere better — and only once the
  // sheet has actually been open: this effect also runs on MOUNT, where a rail
  // arriving with the level's first row already focused (a walk that crossed into
  // or out of a section mounts a new rail) would have that focus taken straight
  // off it and put on More.
  const sheetOpened = useRef(false);
  useEffect(() => {
    if (sheetOpen) {
      sheetOpened.current = true;
      nav.current?.querySelector<HTMLElement>(".navwrap .navitem")?.focus();
      return;
    }
    if (sheetOpened.current && nav.current?.contains(document.activeElement)) {
      sheetOpened.current = false;
      more.current?.focus();
    }
  }, [sheetOpen]);

  // The sheet exists only at phone width — the control that closes it is not
  // rendered above the breakpoint. Widening the window while it is open would
  // otherwise leave the page locked and inert with nothing on screen to release
  // it. The width is already subscribed to above, so this reads that answer
  // rather than opening a second media query of its own.
  useEffect(() => {
    if (sheetOpen && !phone) {
      setSheetOpen(false);
    }
  }, [sheetOpen, phone]);

  // A scrim that only LOOKS blocking is the worst of both worlds: the page it
  // dims stays reachable by Tab and by a screen reader. `inert` on the content
  // column takes it out of the tab order, out of the accessibility tree and out
  // of pointer reach in one attribute — the same guarantee <dialog> gives, which
  // this nav cannot become without giving up its landmark. The body stops
  // scrolling for the same reason: a touch that starts on the scrim should
  // dismiss, not scroll the page underneath.
  //
  // The SKIP LINK goes inert with it, and is not an afterthought: it is a sibling
  // of the nav rather than a child of the column, so Tab past the sheet's last row
  // wrapped onto it, drew it over the scrim, and pointed it at an inert element —
  // a control that takes focus, shows itself, and does nothing.
  useEffect(() => {
    if (!sheetOpen) {
      return;
    }
    const locked = document.querySelectorAll<HTMLElement>(".main, .skiplink");
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    for (const element of locked) {
      element.inert = true;
    }
    return () => {
      document.body.style.overflow = previous;
      for (const element of locked) {
        element.inert = false;
      }
    };
  }, [sheetOpen]);

  // A nav destination that the phone bar hides behind More: on those routes More
  // is the current tab, since the row that would carry the state is not rendered.
  const inSheet = NAV.some(
    (item) => item.screen === route.screen && !MOBILE_PRIMARY.has(item.screen),
  );

  // The agent is APP-level chrome: there is one of it, and it belongs to the
  // whole session rather than to any destination. A drilled-in level is
  // navigation INSIDE one destination, so an agent standing under it would read
  // as that destination's own — re-parented under a sub-level, and a second one
  // the moment a reader walks back out.
  const leveled = level.parent !== undefined;

  return (
    <>
      {/* The scrim dims the page the sheet covers and gives the eye the layer
          boundary. It carries no behaviour of its own: a click on it lands
          outside the nav, which is already what closes the sheet. */}
      {sheetOpen && <div className="railscrim" aria-hidden="true" />}
      <nav
        ref={nav}
        className={railClasses({ collapsed, sheetOpen, leveled })}
        aria-label={t("shell.railAria")}
        onKeyDown={dismissTip}
      >
        <div className="railhead">
          <BrandBlock />
          {/* TEMPORARY, and the whole element goes when the product leaves
              alpha: this span, its rule in shell.css, the `shell.alpha` key and
              the case in rail.test.tsx, together. A ribbon across the head's
              top-left corner, absolutely positioned so it takes no space and
              moves nothing — the head is the same box with it and without it —
              and anchored to the corner rather than to the wordmark, which is
              what keeps it on screen at 64px where every label is gone. */}
          <span className="alphamark">{t("shell.alpha")}</span>
        </div>
        {/* Keyed by depth so a level that arrives is a new element and plays its
            entrance; two addresses at the SAME depth are the same level with
            another row current, and nothing should move. */}
        <NavLevelView
          key={level.depth}
          level={level.shown}
          parent={level.parent}
          counts={counts}
          state={{ collapsed, tip, onTip: setTip }}
          onSelect={level.onSelect}
          onWalkUp={level.onWalkUp}
          // At phone width the agent stands in the MIDDLE of the bar rather
          // than at the foot of a column that is not there — inside the row
          // stream, so a thumb and a Tab key read the bar in the same order.
          // One Core either way: the foot below renders only above the
          // breakpoint.
          centre={phone ? <AgentRail route={route} bar={nav} /> : undefined}
        />
        {/* Phone-width only: expands the bar into a sheet carrying every
          destination. Hidden by CSS on the desktop sidebar.
          It carries the active state for every destination it hides, so the
          closed bar always shows where you are — the four tabs cannot, because
          those routes' own rows are display:none at this width. */}
        <button
          type="button"
          ref={more}
          className={inSheet ? "railmore active" : "railmore"}
          // Open, this control closes the sheet — so it says so, in the name as
          // well as in the glyph. A control whose name stays the same while its
          // job changes is the one a screen reader gets wrong.
          aria-label={sheetOpen ? t("shell.closeMenu") : t("shell.more")}
          aria-expanded={sheetOpen}
          // The state has to reach a screen reader, not just the eye: the hidden
          // route's own link is out of the accessibility tree at this width, so
          // without this nothing in the bar reports the current page. Dropped once
          // the sheet is open, because the real row is then visible and carrying
          // it — two elements claiming the current page is worse than none.
          aria-current={inSheet && !sheetOpen ? "page" : undefined}
          onClick={() => setSheetOpen((open) => !open)}
        >
          {sheetOpen ? <X aria-hidden /> : <Menu aria-hidden />}
          <span className="navlabel">
            {sheetOpen ? t("shell.closeMenu") : t("shell.more")}
          </span>
        </button>
        <div className="grow" />
        {/* The sidebar's foot is the agent. It is the one thing in the chrome
            that reports rather than navigates, and it never claims the current
            page. What the installation is ENTITLED to used to sit here as its
            own grey row, and it now reaches a reader through the Core instead: a
            licence fault turns the orb amber, which is a thing somebody notices.
            The whole foot goes on a drilled-in level, element and all: an empty
            box left behind would still hold the band and the rule that divide a
            reading from the rows above it. And it goes at phone width, where
            there is no column to have a foot: the bar's centre cell above is
            where the agent stands there, and two of these would be two Cores. */}
        {!leveled && !phone && (
          <div className="railagent">
            <AgentRail route={route} />
          </div>
        )}
      </nav>
    </>
  );
}

/**
 * The rail on a settings route, carrying the settings level.
 *
 * Which settings entries exist for a principal is a GRANT question, and grants
 * belong to the screen whose cards ask for them (screens/settings.tsx) — the
 * shell only ever receives a finished section. Mounted on that route alone, so
 * no other screen pays for the visibility probes the hook makes.
 */
export function SettingsRail(props: Readonly<RailProps>) {
  const section = useSettingsSection(props.route);
  return <WorkspaceRail {...props} section={section} />;
}

/**
 * The chrome on a settings route, naming the tab the reader opened.
 *
 * Both halves read the same section the rail does — the hook derives it from the
 * capability cache the rail already warmed, so the trail, the heading and the
 * sidebar cannot disagree about which tab is current.
 */
function SettingsTopBar({
  route,
  collapsed,
  onToggle,
  onOpenSearch,
}: Readonly<{
  route: Route;
  collapsed: boolean;
  onToggle: () => void;
  onOpenSearch: () => void;
}>) {
  const section = useSettingsSection(route);
  return (
    <TopBar
      route={route}
      section={section}
      collapsed={collapsed}
      onToggle={onToggle}
      onOpenSearch={onOpenSearch}
    />
  );
}

function SettingsPageTitle({ route }: Readonly<{ route: Route }>) {
  const section = useSettingsSection(route);
  return <PageTitle route={route} section={section} />;
}

// One group of the switcher's list, with the section's own heading above it — the
// same grouping the sidebar's level shows, because it is the same data.
function SectionPickGroup({
  section,
  group,
  activeId,
  onPick,
}: Readonly<{
  section: NavSection;
  group: NavLevelGroup;
  activeId: string;
  onPick: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="sectionpickgroup">
      {group.headingKey && <h3>{t(group.headingKey)}</h3>}
      {group.items.map((entry) => (
        <a
          key={entry.id}
          href={navEntryHref([section.screen], entry)}
          aria-current={entry.id === activeId ? "page" : undefined}
          // The sheet covers the page it just navigated to, so a row that acts
          // takes the sheet with it.
          onClick={onPick}
        >
          <entry.icon size={16} aria-hidden />
          {entryLabel(entry, locale, t)}
        </a>
      ))}
    </div>
  );
}

/**
 * The way between a section's pages at phone width.
 *
 * The sidebar cannot carry them there — it is a bar of four destinations, and
 * handing those four to a section loses the whole product's navigation — so the
 * section lives in the page head instead: a control naming the entry you are on,
 * opening the section's own list.
 *
 * It opens the design system's `Modal`, which IS the full-screen sheet at this
 * width; a second sheet hand-rolled here would be a second set of dismissal,
 * focus and scroll-lock rules to keep in step with it.
 *
 * The LIST claims the current page and the button does not: the entries are the
 * section's navigation, the same links the sidebar's level carries above the
 * breakpoint, while the button is a control that opens them — and a control is
 * not a page. Several elements may name the current page (the trail's last stop
 * and the sidebar's active row both do, and agree); what none of them may do is
 * claim it for something that is not a destination.
 */
function SectionSwitcher({
  section,
  entry,
}: Readonly<{ section: NavSection; entry: NavLevelEntry }>) {
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const close = useCallback(() => setOpen(false), []);
  const label = entryLabel(entry, locale, t);
  return (
    <>
      <button
        type="button"
        className="pageswitch"
        aria-haspopup="dialog"
        aria-expanded={open}
        // The visible word is the entry the reader is on; the accessible name
        // adds what the control does and keeps that word inside it, which is
        // what WCAG 2.5.3 asks of a control named longer than it reads.
        aria-label={t("shell.sectionSwitch", { name: label })}
        onClick={() => setOpen(true)}
      >
        <span>{label}</span>
        <ChevronDown size={16} aria-hidden />
      </button>
      <Modal open={open} onClose={close} labelledBy={titleId}>
        {/* Named by the SECTION: the list is everything Settings holds, and the
            entry the reader came from is marked inside it. */}
        <h2 id={titleId} className="t-h2">
          {t(section.titleKey)}
        </h2>
        <div className="sectionpick">
          {section.groups.map((group, index) => (
            <SectionPickGroup
              key={group.headingKey ?? `group-${index}`}
              section={section}
              group={group}
              activeId={entry.id}
              onPick={close}
            />
          ))}
        </div>
        {/* At this width the dialog is a full-screen sheet: there is no backdrop
            left to click and a touch reader has no Escape, so the way out has to
            be a control in the sheet. */}
        <div className="actions">
          <Button small onClick={close}>
            {t("shell.closeMenu")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

/**
 * The page's own name, standing in the content column above the content it
 * names.
 *
 * It is INSIDE the scroller and scrolls away with the page, because the heading
 * belongs to the document rather than to the chrome: the top bar says where you
 * are and stays, this says what this page is and goes when you have read it.
 *
 * No page is named twice. A record's surface prints its own identity block and a
 * composed unit names its own screen, so this yields to both — a second name at
 * heading level would leave a screen reader picking between two page titles.
 */
export function PageTitle({
  route,
  section,
}: Readonly<{
  route: Route;
  section?: NavSection;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const phone = usePhoneViewport();

  const navItem = NAV.find((item) => item.screen === route.screen);
  const inSection = sectionHead(section, route);
  // On a screen that publishes a level, the page is the ENTRY the reader opened
  // rather than the section they opened it from: the section is named by the
  // trail in the top bar and by the sidebar's level, and printing it here too
  // named the section twice and the surface never — a settings page read
  // "Settings" above a heading reading "Settings" with the audit log under both.
  //
  // At every width, now. The heading used to name the SECTION at phone width,
  // because the sidebar shows destinations there and nothing else on screen said
  // which section the page belonged to. The trail says it at every width, so the
  // swap left the phone naming the section twice and the entry twice.
  const entryTitle = inSection
    ? entryLabel(inSection.entry, locale, t)
    : undefined;
  const title = entryTitle ?? resolveTitle(route.screen, navItem?.labelKey, t);
  // A record kind, and only then: an id segment that names no record is a
  // screen's own state — the settings tab, for one — and the page is still the
  // screen. Printing that slug as the page's name gave Settings an h1 reading
  // "privacy".
  const recordNamesPage =
    route.id !== undefined && SCREEN_ENTITY[route.screen] !== undefined;
  // Conditioned on the DESCRIPTOR resolving, not on the screen slug alone. A
  // unit route is deliberately absent from both the NAV rail and
  // OFF_RAIL_TITLE_KEYS, so resolveTitle falls through to shell.unknownPage —
  // and a unit this installation did not compose is genuinely an unknown page,
  // whose screen says so in words. That case keeps the heading rather than
  // yielding to a surface that will not name itself either.
  const unitNamesPage =
    route.screen === EXTENSION_SCREEN && findExtension(route.id) !== null;
  // Read only on the branch that prints an h1: a surface that names itself gets
  // no subtitle from here either, or the page would carry a description of a
  // heading it is not showing.
  const subKey = PAGE_SUB_KEYS[route.screen];

  if (recordNamesPage || unitNamesPage) {
    return null;
  }

  // At phone width a section's page IS its switcher: the control names the page
  // and opens the others, so it stands AS the heading rather than under a
  // heading repeating it. The page is still named exactly once at heading level,
  // and the trail above it is the only other place the name appears — the same
  // arithmetic as every other route.
  const switcher = phone && inSection && (
    <SectionSwitcher section={inSection.section} entry={inSection.entry} />
  );

  return (
    <div className="pagetitle">
      <div className="pagetitle-text">
        {/* The heading is named by the PAGE even when a control is all it
            contains. Name-from-content would otherwise reach into the switcher's
            own `aria-label` — "Privacy & audit — change section" — and put an
            instruction in the document's heading list. The control keeps that
            name; the heading states the page. */}
        <h1
          className={switcher ? "t-display pageswitchhead" : "t-display"}
          aria-label={switcher ? title : undefined}
        >
          {switcher || title}
        </h1>
        {subKey && <p className="pagesub">{t(subKey)}</p>}
      </div>
    </div>
  );
}

// The work column's own classes. A RECORD reads wider than a settings page or
// the home screen: it carries a header, a tab strip and two columns under them,
// where the others are one column of prose-width cards. Two widths, and the
// class is what says which — see --pageColumn / --recordColumn.
function mainClasses(gridded: boolean, griddedRecord: boolean): string {
  if (!gridded) {
    return "main";
  }
  return griddedRecord ? "main main-gridded main-record" : "main main-gridded";
}

export function Shell({
  children,
  counts,
  onOpenSearch,
}: Readonly<{
  children: ReactNode;
  counts?: ShellCounts;
  onOpenSearch: () => void;
}>) {
  const route = useRoute();
  const railless = RAIL_LESS_SCREENS.has(route.screen);
  // An extension unit's page is REACHED from settings and says so in its trail
  // (`Settings / <unit>`), so it keeps the settings level in the sidebar. It did
  // not: the rail fell back to the destinations, and following "Open" from a
  // settings card swapped the whole sidebar out from under a reader whose URL
  // and breadcrumb still said Settings. `activeRowFor` in app/nav.ts has always
  // answered `settings` for a unit route; this is the other half of that answer.
  //
  // Conditioned on the descriptor RESOLVING, like the page title's own branch:
  // `#/ext/nonesuch` is a genuinely unknown page and belongs to nothing.
  const onUnitPage =
    route.screen === EXTENSION_SCREEN && findExtension(route.id) !== null;
  const leveled = route.screen === SETTINGS_SCREEN || onUnitPage;
  // Record pages only: the id is what makes it one. `#/companies` is the list,
  // and a list belongs to the other family.
  const griddedRecord =
    route.id !== undefined && GRIDDED_RECORD_SCREENS.has(route.screen);
  // The id-less half of the same policy: a screen that reads down but is not a
  // record, so there is no id to key on. Home is the one today.
  const griddedScreen = GRIDDED_SCREENS.has(route.screen);
  // A unit is NOT in this family, though it is leveled: the reading column is a
  // claim about the page's own content, and a unit's surface is the unit's to
  // lay out.
  const gridded =
    route.screen === SETTINGS_SCREEN || griddedRecord || griddedScreen;
  const [collapsed, setCollapsed] = useState(
    () => readStored(COLLAPSE_KEY) === "1",
  );
  // What the sidebar's walk between levels remembers — where a walk OUT of a
  // level returns to, and whether the level that arrives was asked for and takes
  // focus. The shell holds it because the shell outlives every rail: the rail on
  // a section route is a different component, mounted on the way in and gone
  // again on the way out.
  const walk = useNavWalk(route, !railless && !leveled);
  const scroller = useRef<HTMLDivElement>(null);
  // A route change opens a different page, and a page opens at its top. Nothing
  // in the browser does this for us here: the document itself never scrolls
  // (`.app` is exactly one viewport tall), the content COLUMN does, and that
  // column is the same element on every route — so it carries the offset the
  // last page was left at straight into the next one. Reading a scrolled AI
  // settings page and then opening another entry landed the reader partway down
  // whatever the new page happened to have at that offset, and fewer, longer
  // settings pages made it worse.
  //
  // Keyed by the ADDRESS, not by `route`: useRoute parses a fresh object every
  // render, so a dependency on the object would re-run this on every keystroke a
  // screen handles and fight a reader who has scrolled deliberately.
  // Assigning scrollTop rather than calling scrollTo: it is the same instant jump,
  // it is what the list surface's own reset already uses, and it is a property
  // every environment the tests run in actually has.
  const address = routeHash(route);
  // A new page opens at its top; a page the reader is RETURNING to opens where
  // they left it. Both live in app/scrollmemory.ts, because the second one needs
  // an identity for the history entry that the browser does not give an entry,
  // and a retry while the column grows — a list restores its rows a moment after
  // the address arrives, so a single assignment lands against a column that is
  // still short and gets clamped.
  useScrollMemory(scroller, address);

  const toggle = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      writeStored(COLLAPSE_KEY, next ? "1" : "0");
      return next;
    });
  }, []);

  // ⌘B / Ctrl B, the shortcut the toggle's own tooltip has advertised since the
  // sidebar had a toggle — and which nothing bound, so the tooltip named a key
  // that did nothing. Registered here because this is where the state lives.
  // Ignored while a text field has focus: the same chord is bold in every editor
  // on the platform, and a composer is exactly where a reader would reach for it.
  useEffect(() => {
    function onKeyDown(event: globalThis.KeyboardEvent) {
      // The chord EXACTLY: one platform modifier and no others. Accepting any
      // combination that merely included Meta or Ctrl took Cmd+Shift+B — which
      // is bold-with-a-selection in most editors, and something of its own in
      // the browser — and swallowed it with preventDefault.
      const platformKey = event.metaKey !== event.ctrlKey;
      if (
        !platformKey ||
        event.altKey ||
        event.shiftKey ||
        event.key.toLowerCase() !== "b"
      ) {
        return;
      }
      // `instanceof` rather than an assertion: an event target is an
      // `EventTarget`, and focus genuinely can sit on an SVGElement, which has
      // no `isContentEditable` at all.
      const target = event.target;
      if (
        target instanceof HTMLElement &&
        (target.isContentEditable || /^(INPUT|TEXTAREA)$/.test(target.tagName))
      ) {
        return;
      }
      event.preventDefault();
      toggle();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [toggle]);

  useEffect(() => {
    document.body.dataset.screen = route.screen;
  }, [route.screen]);

  if (railless) {
    return (
      <div className="app railless">
        {/* No skip link here, and that is not an omission: these routes carry no
            navigation, so there is no repeated block ahead of the content for a
            reader to skip past (WCAG 2.4.1). */}
        <main className="main">
          <div className="scroll" ref={scroller}>
            {children}
          </div>
        </main>
      </div>
    );
  }

  const railProps: RailProps = {
    route,
    counts,
    collapsed,
  };

  return (
    // The provider spans the whole chrome, because the column and the screen
    // that fills it are on opposite sides of the tree: the region is a sibling
    // of <main>, the content comes from a screen inside it.
    <PageAsideProvider>
      <div className={collapsed ? "app" : "app railexpanded"}>
        <SkipToContent target={scroller} />
        {/* One rail, two suppliers of what it shows: a screen owning a level wires
          its own data in, everything else renders the destinations alone. The
          provider spans both, because the way out of a level is asked for on the
          rail that has one and answered with what the other one saw. */}
        <NavWalkProvider value={walk}>
          {leveled ? (
            <SettingsRail {...railProps} />
          ) : (
            <WorkspaceRail {...railProps} />
          )}
        </NavWalkProvider>
        <main className={mainClasses(gridded, griddedRecord)}>
          {leveled ? (
            <SettingsTopBar
              route={route}
              collapsed={collapsed}
              onToggle={toggle}
              onOpenSearch={onOpenSearch}
            />
          ) : (
            <TopBar
              route={route}
              collapsed={collapsed}
              onToggle={toggle}
              onOpenSearch={onOpenSearch}
            />
          )}
          {/* Public, onboarding, and preference routes are intentionally
            railless; these advisories belong only here. */}
          <EconomyBanner />
          <EmbedReindexBanner />
          {/* Focusable only as the skip link's destination — never a tab stop of
            its own, which is what tabIndex -1 buys. A reader who takes the skip
            lands here, PAST the strip, and the next Tab continues into the page's
            own controls. */}
          <div className="scroll" ref={scroller} tabIndex={-1}>
            {leveled ? (
              <SettingsPageTitle route={route} />
            ) : (
              <PageTitle route={route} />
            )}
            {children}
          </div>
        </main>
        {/* The page's context column — a column of the WINDOW, beside the work
          rather than inside it, so it runs the full height past the page's own
          header and does not move when a tab changes. A screen that fills none
          leaves nothing here. */}
        <PageAsideRegion />
        {/* The agent's own periphery, drawn around the WHOLE workspace rather than
          around the content column: what it reports is true of the window a
          person is working in, and a contour that stopped at the sidebar would
          read as a panel border. Last in the tree, because it is an overlay and
          not a column. */}
        <AgentEdge />
      </div>
    </PageAsideProvider>
  );
}

export { navigate } from "./router";
export { useRoute };
