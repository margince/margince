import {
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { PageAsideProvider } from "../components/pageaside";
import { PageHeadSlot } from "../components/pageheadaction";
import { ComposeDockProvider } from "../features/mail/composedock";
import { SETTINGS_SCREEN } from "../features/settings/settingsnav";
import { useT } from "../i18n";
import { EXTENSION_SCREEN, findExtension } from "../lib/extensions";
import { useFirstScreenSettled } from "../lib/journeymarks";
import { useLiveHash, useRoute } from "../lib/router";
import { ShownAddressSink } from "../lib/screenaddress";
import { useScrollMemory } from "../lib/scrollmemory";
import { useShortcutDispatcher, useShortcutHandler } from "../lib/shortcuts";
import { readStored, STORAGE_KEYS, writeStored } from "../lib/storage";
import { PHONE_MAX_WIDTH, widthMedia } from "../lib/viewport";
import { AgentEdge } from "./agent-edge";
import { CaptureChip } from "./capture-chip";
import { CaptureHealthBanner } from "./capturehealthbanner";
import { ConnectivityBanner } from "./connectivitybanner";
import { EconomyBanner } from "./economybanner";
import { EmbedReindexBanner } from "./embedreindexbanner";
import { LicenseBanner } from "./licensebanner";
import { type NavCounts, RAIL_LESS_SCREENS } from "./nav";
import { NavWalkProvider, useNavWalk } from "./navlevel";
import { useReadingColumn } from "./readingcolumn";
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
import { PageTitle, SettingsPageTitle } from "./shelltitle";

export { SettingsRail, WorkspaceRail } from "./shellrail";
export { PageTitle } from "./shelltitle";

// The attention counts the rail badges on the rows a level declares badgeable.
// They are the levels' own currency (app/subnav.ts), named here for the shell
// because this is the seam a caller hands them in at. No caller does today: the
// primary level badges nothing (app/nav.ts BADGE_SCREENS) now the queues that
// had counts are lanes inside Today, which reports its numbers on the page. The
// prop stays because a deeper level declaring `badgeIds` needs this door.
export type ShellCounts = NavCounts;

// WCAG 2.4.1, Bypass Blocks. Every page puts the sidebar and the top bar ahead
// of its content, and without this a keyboard reader walks them on every page.
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

// The work column's own classes. `main-gridded` is the capped reading column;
// `main-record` adds no width and names the column's SCROLLER as a container,
// so the record's tab strip measures the width content can use (shell.css,
// `container-name: work`).
const RAIL_FOLDS_MAX_WIDTH = 1024;

// A stored choice wins; with none a tablet starts folded, and nothing is stored.
function startsCollapsed(): boolean {
  const stored = readStored(STORAGE_KEYS.sidebarCollapsed);
  if (stored !== null) {
    return stored === "1";
  }
  const under = (width: number) =>
    globalThis.matchMedia?.(widthMedia(width)).matches ?? false;
  return under(RAIL_FOLDS_MAX_WIDTH) && !under(PHONE_MAX_WIDTH);
}

function mainClasses(gridded: boolean, record: boolean): string {
  return ["main", gridded && "main-gridded", record && "main-record"]
    .filter(Boolean)
    .join(" ");
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
  // not: the rail fell back to the destinations, and following "Open" on a
  // settings card swapped the whole sidebar out from under a reader whose URL
  // and breadcrumb still said Settings. `activeRowFor` in app/nav.ts has always
  // answered `settings` for a unit route; this is the other half of it.
  //
  // Conditioned on the descriptor RESOLVING, like the page title's own branch:
  // `#/ext/nonesuch` is a genuinely unknown page and belongs to nothing.
  const onUnitPage =
    route.screen === EXTENSION_SCREEN && findExtension(route.id) !== null;
  const leveled = route.screen === SETTINGS_SCREEN || onUnitPage;
  const { gridded, record } = useReadingColumn(route);
  const [collapsed, setCollapsed] = useState(startsCollapsed);
  // What the sidebar's walk between levels remembers — where a walk OUT of a
  // level returns to, and whether the level that arrives was asked for and takes
  // focus. The shell holds it because the shell outlives every rail: the rail on
  // a section route is a different component, mounted on the way in and gone
  // again on the way out.
  const walk = useNavWalk(route, !railless && !leveled);
  const [headSlot, setHeadSlot] = useState<HTMLElement | null>(null);
  const scroller = useRef<HTMLDivElement>(null);
  // A page opens at its top. The document never scrolls; the content column
  // does, and it is one element on every route, so it would carry the old offset.
  //
  // Keyed by the ADDRESS the routed screen shows, not the bar: a screen held
  // past a move is still the one in the column. The whole address, so Back
  // between two entries that differ only in dials restores each.
  const [shown, setShown] = useState<string | null>(null);
  const live = useLiveHash();
  const address = shown ?? live;
  // A new page opens at its top; a page the reader is returning to opens where
  // they left it. Both live in lib/scrollmemory.ts. The second needs an identity
  // for the history entry, which the browser does not give, and a retry while the
  // column grows. A list restores its rows a moment after the address arrives,
  // so a single assignment lands on a column still short and gets clamped.
  useScrollMemory(scroller, address);
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
          {route.screen === "onboarding" && <CaptureChip linked={false} />}
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
    onToggle: toggle,
  };

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
          <main className={mainClasses(gridded, record)}>
            <TopBar route={route} onOpenSearch={onOpenSearch} />
            {/* Mounted from the start: it holds the outage watcher, and a
                known offline state shows in the first paint. */}
            <ConnectivityBanner />
            <ShellBanners />
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
function ShellBanners() {
  if (!useFirstScreenSettled()) return null;
  return (
    <>
      <CaptureHealthBanner />
      <EconomyBanner />
      <EmbedReindexBanner />
      <LicenseBanner />
    </>
  );
}
