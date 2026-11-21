import {
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { useEntityName } from "../components/entityref.queries";
import { PageAsideProvider } from "../components/pageaside";
import { PageHeadSlot } from "../components/pageheadaction";
import { PageWait, usePageWait } from "../design-system/pagewait";
import { ComposeDockProvider } from "../features/mail/composedock";
import {
  SETTINGS_SCREEN,
  useSettingsSection,
} from "../features/settings/settingsnav";
import { useLocale, useT } from "../i18n";
import { EXTENSION_SCREEN, findExtension } from "../lib/extensions";
import { useFirstScreenSettled } from "../lib/journeymarks";
import { routeIdentity } from "../lib/routeidentity";
import { parseHash, type Route, useLiveHash, useRoute } from "../lib/router";
import { ShownAddressSink, type ShownScreen } from "../lib/screenaddress";
import { useScrollMemory } from "../lib/scrollmemory";
import { useShortcutDispatcher, useShortcutHandler } from "../lib/shortcuts";
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import { AgentEdge } from "./agent-edge";
import { CaptureChip } from "./capture-chip";
import { CaptureHealthBanner } from "./capturehealthbanner";
import { ConnectivityBanner } from "./connectivitybanner";
import { EconomyBanner } from "./economybanner";
import { EmbedReindexBanner } from "./embedreindexbanner";
import { InstallHint } from "./installhint";
import { LicenseBanner } from "./licensebanner";
import {
  entryLabel,
  type NavCounts,
  type NavSection,
  RAIL_LESS_SCREENS,
  recordKindOf,
} from "./nav";
import { NavWalkProvider, useNavWalk } from "./navlevel";
import { sectionHead, usePageName, useRouteSubject } from "./pagemeta";
import { isRecordPage, readingColumn } from "./readingcolumn";
import { usePublishedScrollerHeight } from "./scrollerheight";
import { TopBar } from "./topbar";
import "./shell.css";

// The app shell: one sidebar against the left edge of the viewport, and the
// content beside it. Collapsed the sidebar is the canonical 56px rail WDS-NAV-1
// specifies, unchanged — the labeled state is additive, so the spec stays true
// at 56px rather than being replaced.
//
// The chrome is an L: the sidebar carries where you can GO, and the top bar
// above the content (app/topbar.tsx) carries everything else that is true of the
// whole session — where you are, how you search, and who you are signed in as.
// The sidebar holds destinations and nothing else, because a panel that also
// held the search, the settings door and the contact had four different kinds of
// row in one column and read as a list of everything.
//
// The content column carries only what is true of THIS screen: its heading,
// which stands INSIDE the scroller and scrolls with the page because it belongs
// to the document, and the agent, which floats at the column's foot.
import { type RailProps, SettingsRail, WorkspaceRail } from "./shellrail";
import { PageTitle, SettingsPageTitle, SettingsTopBar } from "./shelltitle";

export { SettingsRail, WorkspaceRail } from "./shellrail";
export { PageTitle } from "./shelltitle";

// The attention counts the rail badges on the rows a level declares badgeable.
// They are the levels' own currency (app/subnav.ts), named here for the shell
// because this is the seam a caller hands them in at. No caller does today: the
// primary level badges nothing (app/nav.ts BADGE_SCREENS) now the queues that
// had counts are lanes inside Today, which reports its numbers on the page. The
// prop stays because a deeper level declaring `badgeIds` needs this door.
export type ShellCounts = NavCounts;

// WCAG 2.4.1, Bypass Blocks. Every page in the product puts the same block ahead
// of its content — up to twelve navigation rows and the agent in the sidebar,
// then the strip's collapse control, trail, search, system-of-record
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

// The work column's own classes. `main-gridded` is the capped reading column and
// there is one measure for every page that keeps it (--pageColumn);
// `main-record` adds nothing to that width and never appears without it — it
// names the column's SCROLLER as a container, so the record's tab strip
// measures the width content can use (shell.css, `container-name: work`).
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
  held = false,
}: Readonly<{
  children: ReactNode;
  counts?: ShellCounts;
  onOpenSearch: () => void;
  /** The onboarding gate holds the reader on the few pages it lets them see. */
  held?: boolean;
}>) {
  const live = useRoute();
  const liveHash = useLiveHash();
  const [shown, setShown] = useState<ShownScreen | null>(null);
  // The chrome names the page whose content is on screen. It lags the bar while
  // the next page loads or a draft holds this one.
  const route = shown ? parseHash(shown.address) : live;
  const railless = RAIL_LESS_SCREENS.has(live.screen);
  const leveled = isLeveled(route);
  const { gridded, griddedRecord } = readingColumn(route);
  const waiting = usePageWait(shown?.pending ?? false);
  const [collapsed, setCollapsed] = useState(
    () => readStored(STORAGE_KEYS.sidebarCollapsed) === "1",
  );
  // What the sidebar's walk between levels remembers — where a walk OUT of a
  // level returns to, and whether the level that arrives was asked for and takes
  // focus. The shell holds it because the shell outlives every rail: the rail on
  // a section route is a different component, mounted on the way in and gone
  // again on the way out.
  const walk = useNavWalk(live, !railless && !isLeveled(live));
  const [headSlot, setHeadSlot] = useState<HTMLElement | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // A page opens at its top. The document never scrolls; the content column
  // does, and it is one element on every route, so it would carry the old offset.
  //
  // Keyed by the ADDRESS the routed screen shows, not the bar: a screen held
  // past a move is still the one in the column. The whole address, so Back
  // between two entries that differ only in dials restores each.
  const address = shown?.address ?? liveHash;
  // A new page opens at its top and a page met again where it was left. Another
  // tab of the record on screen keeps the reader's place.
  useScrollMemory(scroller, address, { view: recordShown(address) });
  const screen = (
    <ShownAddressSink.Provider value={setShown}>
      {children}
    </ShownAddressSink.Provider>
  );
  usePublishedScrollerHeight(scroller, railless ? "railless" : "workspace");

  const toggle = useCallback(() => {
    setCollapsed((current) => {
      const next = !current;
      writeStored(STORAGE_KEYS.sidebarCollapsed, next ? "1" : "0");
      return next;
    });
  }, []);

  useShortcutHandler("sidebar", toggle);
  useShortcutDispatcher();

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
          {/* The import a reader started during setup keeps running while they
              answer the rest of it, and the step that drew the run is behind
              them by then — so the gauge belongs here too. Onboarding only:
              the other rail-less routes are the anonymous surfaces, which have
              no connections to read, and the consent screen, which is one
              request in flight and nothing else. Unlinked, because these
              routes carry no navigation and a reader mid-flow has a place to
              lose. */}
          {live.screen === "onboarding" && <CaptureChip linked={false} />}
          <div className="scroll" ref={scroller}>
            {screen}
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
  const bar = { route, collapsed, onToggle: toggle, onOpenSearch, held };

  return (
    // The provider spans the whole chrome, because the details pane's memory
    // — open or folded — outlives the screen that draws it: the switch and
    // the pane are the record's, the preference is the reader's.
    <PageAsideProvider>
      {/* The composer docks here rather than on the page that opened it, so a
          message being written survives the reader moving to another page. */}
      <ComposeDockProvider>
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
          <main
            className={mainClasses(gridded, griddedRecord)}
            aria-busy={waiting || undefined}
          >
            {leveled ? <SettingsTopBar {...bar} /> : <TopBar {...bar} />}
            {live.screen === SETTINGS_SCREEN ? (
              <SettingsPageArriving route={live} waiting={waiting} />
            ) : (
              <PageArriving route={live} waiting={waiting} />
            )}
            {/* Mounted from the start: it holds the outage watcher, and a
                known offline state shows in the first paint. */}
            <ConnectivityBanner />
            <ShellBanners scroller={scroller} />
            {/* The import's gauge, floating bottom-centre of this column for as
            long as mail is arriving. Inside `.main` rather than beside the
            edge below, because it is positioned against the content column
            and not the window. */}
            <CaptureChip linked />
            {/* Focusable only as the skip link's destination, never a tab stop of
            its own, which is what tabIndex -1 buys. A reader who takes the skip
            lands here, past the strip, and the next Tab continues into the page's
            own controls. */}
            <div className="scroll" ref={scroller} tabIndex={-1}>
              {leveled ? (
                <SettingsPageTitle route={route} actionSlot={setHeadSlot} />
              ) : (
                <PageTitle route={route} actionSlot={setHeadSlot} />
              )}
              <PageHeadSlot.Provider value={headSlot}>
                {screen}
              </PageHeadSlot.Provider>
            </div>
          </main>
          {/* The agent's own periphery, drawn around the whole workspace rather than
          around the content column: what it reports is true of the window a
          contact is working in, and a contour that stopped at the sidebar would
          read as a panel border. Last in the tree, because it is an overlay and
          not a column. */}
          <AgentEdge />
        </div>
      </ComposeDockProvider>
    </PageAsideProvider>
  );
}

export { navigate } from "../lib/router";
export { useRoute };

// Each banner makes its own read; they wait for the first screen's day.
function ShellBanners({
  scroller,
}: Readonly<{ scroller: RefObject<HTMLElement | null> }>) {
  if (!useFirstScreenSettled()) return null;
  return (
    <>
      <CaptureHealthBanner />
      <EconomyBanner />
      <EmbedReindexBanner />
      <LicenseBanner />
      <InstallHint scroller={scroller} />
    </>
  );
}

// A unit's page is reached from settings and its trail says so, so it keeps the
// settings level. `#/ext/nonesuch` resolves to no unit and is an unknown page.
function isLeveled(route: Route): boolean {
  const onUnitPage =
    route.screen === EXTENSION_SCREEN && findExtension(route.id) !== null;
  return route.screen === SETTINGS_SCREEN || onUnitPage;
}

// A settings page is named as its heading names it.
function SettingsPageArriving(
  props: Readonly<{ route: Route; waiting: boolean }>,
) {
  const section = useSettingsSection(props.route);
  return <PageArriving {...props} section={section} />;
}

// A record whose name has not been read is named by its list, never its id.
function PageArriving({
  route,
  waiting,
  section,
}: Readonly<{ route: Route; waiting: boolean; section?: NavSection }>) {
  const t = useT();
  const { locale } = useLocale();
  const kind = recordKindOf(route);
  const record = kind && route.id;
  const { name } = useEntityName(kind ?? "contact", record);
  const subject = useRouteSubject(route);
  const list = usePageName(route.screen);
  const entry = sectionHead(section, route)?.entry;
  const named = record ? (name ?? list) : subject;
  const page = entry ? entryLabel(entry, locale, t) : named;
  return (
    <PageWait waiting={waiting} label={t("settings.pageLoading", { page })} />
  );
}

// The record a record page shows, whichever tab and dials it has open.
function recordShown(address: string): string | undefined {
  const route = parseHash(address);
  return isRecordPage(route) ? routeIdentity(route) : undefined;
}
