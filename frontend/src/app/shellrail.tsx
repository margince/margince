import { Menu, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { AppLink } from "../design-system/applink";
import { Avatar } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Logomark } from "../design-system/logomark";
import { useTruncationTooltip } from "../design-system/tooltip";
import { useSettingsSection } from "../features/settings/settingsnav";
import { useT } from "../i18n";
import { type Route, routeHash } from "../lib/router";
import { useMe } from "../lib/session";
import { usePhoneViewport } from "../lib/viewport";
import { AgentRail } from "./agentrail";
import { currentClaim, MOBILE_PRIMARY, NAV, type NavSection } from "./nav";
import { NavLevelView, useNavLevel } from "./navlevel";
import { useNavTip } from "./navtip";
import { usePopoverDismiss } from "./popover";
import type { ShellCounts } from "./shell";
import { useSquareMark } from "./squaremark";

// The installation's own name and marks (ADR-0061: one installation, one
// company). They come from /me, which carries them to every seat. The full
// company profile is an admin's read. A disabled observer: the auth gate
// already fetched /me, and this re-renders when a company save refreshes it.
function useInstallationBrand() {
  return useMe(false).data?.installation_brand;
}

/** The accessible name of a link home that shows the company's mark. */
export function useHomeLinkLabel(): string {
  const t = useT();
  const installation = useInstallationBrand();
  return installation
    ? t("shell.companyLogoAria", { company: installation.display_name })
    : t("shell.logoAria");
}

/**
 * The company's square mark, or the product's own before onboarding named a
 * company. Decorative: the link around it carries the name.
 */
export function WorkspaceChip() {
  const installation = useInstallationBrand();
  const mark = useSquareMark(
    installation?.logo_icon_url,
    installation?.logo_url,
  );
  if (!installation) {
    return (
      <span className="ws-chip" aria-hidden="true">
        <Logomark />
      </span>
    );
  }
  // Avatar draws the deterministic monogram underneath, so a company with no
  // square mark has a face, not a gap.
  return (
    <span className="ws-chip ws-chip-company" aria-hidden="true">
      {/* The brand read carries no company id, so the name keys the chip. */}
      <Avatar
        identity={installation.display_name}
        name={installation.display_name}
        src={mark}
      />
    </span>
  );
}

// One lockup at every width: a square mark and the name. Once the installation
// has a name of its own, what it runs on sits under it.
// A square is the one shape every company's mark fits at one weight.
// Only the name is the link, and the mark repeats it, so the mark is hidden
// from assistive tech. The link covers the whole card (shell.css).
function BrandBlock() {
  const t = useT();
  const installation = useInstallationBrand();
  const homeLabel = useHomeLinkLabel();
  // On the link, not the heading: the link already holds the tab stop, and the
  // measure reaches into the heading it clips. Folded, the name has no width,
  // so hovering the mark always shows it.
  const {
    ref: nameRef,
    trigger: nameTrigger,
    tip: nameTip,
  } = useTruncationTooltip<HTMLAnchorElement>(installation?.display_name ?? "");
  // With no profile (the shell mounted before onboarding described anybody),
  // the block falls back to the product's own mark and name. A company name is
  // never invented to fill the slot.
  if (!installation) {
    return (
      <div className="railhead">
        <WorkspaceChip />
        <AppLink className="ws" href="#/home" aria-label={homeLabel}>
          <Heading size="small" as="div" className="ws-name">
            {t("shell.logoAria")}
          </Heading>
        </AppLink>
      </div>
    );
  }
  return (
    <div className="railhead railhead-attributed">
      <WorkspaceChip />
      <AppLink
        className="ws"
        href="#/home"
        aria-label={homeLabel}
        ref={nameRef}
        {...nameTrigger}
      >
        {/* `as="div"`: the workspace name is the rail's identity, not a section
            of the page, so an h5 here would be a level nothing opened. */}
        <Heading size="small" as="div" className="ws-name">
          {installation.display_name}
        </Heading>
        {nameTip}
      </AppLink>
      <span className="ws-company">{t("shell.poweredBy")}</span>
    </div>
  );
}

// The panel's state as classes: its width, whether the phone menu is open,
// whether it is mid-collapse, and whether it shows a drilled-in level. The
// last is a class, not a CSS `:has()` on the level's markup. A `:has()` takes
// its argument's specificity, so a rule meant only to arrange a panel would
// have outranked the menu's layout.
function railClasses(
  state: Readonly<{
    collapsed: boolean;
    menuOpen: boolean;
    leveled: boolean;
  }>,
): string {
  return [
    "rail",
    state.collapsed ? "collapsed" : "expanded",
    state.menuOpen ? "menuopen" : "",
    state.leveled ? "leveled" : "",
  ]
    .filter((name) => name !== "")
    .join(" ");
}

export type RailProps = {
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
  const nav = useRef<HTMLElement>(null);
  // Collapsed items are icon-only, so the label needs a WCAG 1.4.13 tooltip.
  // It appears on keyboard focus as well as hover. It stays visible while the
  // pointer is on it (it renders inside the hovered wrapper). Escape dismisses
  // it without moving focus. The tooltip is never the accessible name:
  // aria-label carries that in both states.
  const navTip = useNavTip(collapsed, nav);
  // At phone width the same markup is a bottom bar; More opens a menu over it
  // with the rest. One nav element keeps one navigation
  // landmark and no second item list to keep in sync.
  const [openAt, setOpenAt] = useState<string | undefined>(undefined);
  const here = routeHash(route);
  // Keyed by the address it opened at, so any navigation closes it.
  const menuOpen = openAt === here;
  const phone = usePhoneViewport();
  const more = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);

  // The menu, not the nav: a press on the bar or the agent closes it too.
  const closeMenu = useCallback(() => setOpenAt(undefined), []);
  usePopoverDismiss(menuOpen, menu, closeMenu);

  // A section's entries take over the panel, one level at a time. The phone
  // bar keeps its destinations on a section route, which is reached from the
  // page head there (app/sectionswitcher.tsx).
  const level = useNavLevel(route, phone ? undefined : section, nav, closeMenu);

  // The menu takes focus, or Tab walks a page nobody can see; focus it took down
  // with it returns to More. The ref spares the mount, which keeps its focus.
  const menuOpened = useRef(false);
  useEffect(() => {
    if (menuOpen) {
      menuOpened.current = true;
      menu.current?.querySelector<HTMLElement>(".navitem")?.focus();
      return;
    }
    if (!menuOpened.current) {
      return;
    }
    menuOpened.current = false;
    if (
      document.activeElement === null ||
      document.activeElement === document.body
    ) {
      more.current?.focus();
    }
  }, [menuOpen]);

  // The menu exists only at phone width; the control that closes it is not
  // rendered above the breakpoint. Widening the window while it is open would
  // otherwise leave the page locked and inert with nothing on screen to release
  // it. The width is already subscribed to above, so this reads that answer
  // rather than opening a second media query of its own.
  useEffect(() => {
    if (menuOpen && !phone) {
      setOpenAt(undefined);
    }
  }, [menuOpen, phone]);

  // `inert` takes the dimmed page out of Tab, the accessibility tree and pointer
  // reach. The skip link sits outside `.main`, so it goes inert too, and the
  // body stops scrolling so a touch on the scrim dismisses.
  useEffect(() => {
    if (!menuOpen) {
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
  }, [menuOpen]);

  // A destination the phone bar hides behind More: there More is the current
  // tab, and claims what the row it stands in for would (`currentClaim`).
  const shownRow = NAV.find((item) => item.screen === level.shown.activeId);
  const offBar = shownRow ? !MOBILE_PRIMARY.has(shownRow.screen) : false;
  const moreClaim = offBar ? currentClaim(level.shown) : undefined;

  // The agent is app-level chrome: there is one of it, and it belongs to the
  // whole session rather than to any destination. A drilled-in level is
  // navigation inside one destination. An agent under it would read as that
  // destination's own: re-parented under a sub-level, and a second one the
  // moment a reader walks back out.
  const leveled = level.parent !== undefined;

  return (
    <>
      {/* No behaviour of its own: a press on the scrim lands outside the menu,
          which is what closes it. */}
      {menuOpen && <div className="railscrim" aria-hidden="true" />}
      <nav
        ref={nav}
        className={railClasses({ collapsed, menuOpen, leveled })}
        aria-label={t("shell.railAria")}
      >
        {/* Before the bar in the document as it is above it on screen, so Tab
            meets the menu first (WCAG 2.4.3). */}
        {menuOpen && (
          <div className="railmenu" ref={menu}>
            <NavLevelView
              level={level.shown}
              counts={counts}
              state={navTip}
              onSelect={level.onSelect}
              onWalkUp={level.onWalkUp}
              rows="offbar"
            />
          </div>
        )}
        {!phone && <BrandBlock />}
        {/* Keyed by depth so a level that arrives is a new element and plays its
            entrance; two addresses at the same depth are the same level with
            another row current, and nothing should move. */}
        <NavLevelView
          key={level.depth}
          level={level.shown}
          parent={level.parent}
          counts={counts}
          state={navTip}
          onSelect={level.onSelect}
          onWalkUp={level.onWalkUp}
          rows={phone ? "bar" : "all"}
          // At phone width the agent stands in the middle of the bar, not at
          // the foot of a column that is not there. It sits in the row stream,
          // so a thumb and a Tab key read the bar in the same order. One Core
          // either way: the foot below renders only above the breakpoint.
          centre={phone ? <AgentRail route={route} bar={nav} /> : undefined}
        />
        {/* Carries the active state for every destination the menu holds, so
            the closed bar always shows where you are. */}
        <button
          type="button"
          ref={more}
          className={offBar ? "railmore active" : "railmore"}
          // Open, this control closes the menu, so it says so in the name as
          // well as in the glyph. A control whose name stays the same while its
          // job changes is the one a screen reader gets wrong.
          aria-label={menuOpen ? t("shell.closeMenu") : t("shell.more")}
          aria-expanded={menuOpen}
          // The state has to reach a screen reader as well as the eye: the
          // hidden row is out of the accessibility tree at this width. Dropped
          // once the menu is open, because the real row is then visible and
          // carries it. Two elements claiming the current page is worse than
          // none.
          aria-current={menuOpen ? undefined : moreClaim}
          onClick={() => setOpenAt(menuOpen ? undefined : here)}
        >
          {/* Both glyphs stay drawn, so one can turn into the other. */}
          <span className="railmore-glyphs" aria-hidden>
            <Menu className="railmore-open" />
            <X className="railmore-close" />
          </span>
          <span className="navlabel">
            {menuOpen ? t("shell.closeMenu") : t("shell.more")}
          </span>
        </button>
        <div className="grow" />
        {/* The sidebar's foot is the agent. It is the one thing in the chrome
            that reports rather than navigates, and it never claims the current
            page. It does not carry the licence: the banner over the page states
            a refused one and the agent panel's pill an absent or refused one.
            On a drilled-in level it is reduced rather than removed
            (app/agentrail.css). The agent belongs to the whole session, but the
            column there is one section's own pages. The ball at rail size would
            lead a list it is not about. It goes at phone width, where there is
            no column to have a foot. There the agent stands in the bar's centre
            cell above, and two would be two Cores. */}
        {!phone && (
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
 * Which settings entries exist for a principal is a grant question. Grants
 * belong to the screen whose cards ask for them (features/settings/settings.tsx); the
 * shell only ever receives a finished section. Mounted on that route alone, so
 * no other screen pays for the visibility probes the hook makes.
 */
export function SettingsRail(props: Readonly<RailProps>) {
  const section = useSettingsSection(props.route);
  return <WorkspaceRail {...props} section={section} />;
}
