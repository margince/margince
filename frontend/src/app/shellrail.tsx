import { Menu, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { AppLink } from "../design-system/applink";
import { Avatar } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { Logomark } from "../design-system/logomark";
import { useTruncationTooltip } from "../design-system/tooltip";
import { useSettingsSection } from "../features/settings/settingsnav";
import { useT } from "../i18n";
import type { Route } from "../lib/router";
import { useMe } from "../lib/session";
import { usePhoneViewport } from "../lib/viewport";
import { AgentRail } from "./agentrail";
import { BetaBadge } from "./betabadge";
import { currentClaim, MOBILE_PRIMARY, NAV, type NavSection } from "./nav";
import { NavLevelView, useNavLevel } from "./navlevel";
import { useNavTip } from "./navtip";
import { usePopoverDismiss } from "./popover";
import type { ShellCounts } from "./shell";
import { useSquareMark } from "./squaremark";

// One lockup at every width: a square mark, the name, and what it runs on.
// A square is the one shape every company's mark fits at one weight.
// Only the name is the link, and the mark repeats it, so the mark is hidden
// from assistive tech. shell.css stretches the link over the whole card.
function BrandBlock() {
  const t = useT();
  /* The stage this product is at, on every branch below. It is a fact about
   * the build, not about the company above it.
   * Temporary: betabadge.tsx lists what its deletion takes. */
  const marker = <BetaBadge />;
  // The installation's own name and marks (ADR-0061: one installation, one
  // company). They come from /me, which carries them to every seat. The full
  // company profile is an admin's read. A disabled observer: the auth gate
  // already fetched /me, and this re-renders when a company save refreshes it.
  const installation = useMe(false).data?.installation_brand;
  const mark = useSquareMark(
    installation?.logo_icon_url,
    installation?.logo_url,
  );
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
  // never invented to fill the line.
  if (!installation) {
    return (
      <>
        <span className="ws-chip" aria-hidden="true">
          <Logomark />
        </span>
        <AppLink className="ws" href="#/home" aria-label={t("shell.logoAria")}>
          <Heading size="small" as="div" className="ws-name">
            {t("shell.logoAria")}
          </Heading>
        </AppLink>
        {/* No attribution here: the product's own name is already above it. */}
        <span className="ws-company">{marker}</span>
      </>
    );
  }
  return (
    <>
      {/* The company's square mark. Avatar draws the deterministic monogram
          underneath, so a company with no square mark has a face, not a gap. */}
      <span className="ws-chip ws-chip-company" aria-hidden="true">
        {/* The brand read carries no company id, so the name keys the chip. */}
        <Avatar
          identity={installation.display_name}
          name={installation.display_name}
          src={mark}
        />
      </span>
      <AppLink
        className="ws"
        href="#/home"
        aria-label={t("shell.companyLogoAria", {
          company: installation.display_name,
        })}
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
      <span className="ws-company">
        <span className="ws-company-text">{t("shell.poweredBy")}</span>
        {marker}
      </span>
    </>
  );
}

// The panel's state as classes: its width, whether the phone sheet is open,
// whether it is mid-collapse, and whether it shows a drilled-in level. The
// last is a class, not a CSS `:has()` on the level's markup. A `:has()` takes
// its argument's specificity, so a rule meant only to arrange a panel would
// have outranked the sheet's layout.
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
  // At phone width the same markup is a bottom bar; More expands it into a
  // sheet carrying every destination. One nav element keeps one navigation
  // landmark and no second item list to keep in sync.
  const [sheetOpen, setSheetOpen] = useState(false);
  const phone = usePhoneViewport();
  const more = useRef<HTMLButtonElement>(null);

  // The sheet is a popover over the page. It dismisses like every other one in
  // the chrome: Escape from anywhere, or any click outside it. That makes
  // the scrim behind it work without being a control of its own.
  const closeSheet = useCallback(() => setSheetOpen(false), []);
  usePopoverDismiss(sheetOpen, nav, closeSheet);

  // Which of the route's levels the panel shows, and the two ways the reader
  // moves between them (app/navlevel.tsx). A section's entries take over the
  // panel rather than hanging off the destinations. 56px cannot carry two
  // levels, and 256px carrying both reads as a list of twenty places to go.
  //
  // At phone width the panel is a bottom bar of four destinations, and it
  // keeps them on a section route. A bar that gave its four tabs to a section
  // would lose every destination and hold two controls. There the section is
  // reached from the page head (app/sectionswitcher.tsx), so no section is
  // walked into here.
  const level = useNavLevel(
    route,
    phone ? undefined : section,
    nav,
    closeSheet,
  );

  // Opening a sheet that covers the page takes focus with it, or a keyboard
  // user tabs through a page they cannot see. Closing it hands focus back to
  // the control that opened it, not to <body>. That happens only while the
  // sheet still holds focus; a row that navigated away put focus somewhere
  // better. It also happens only once the sheet has been open. The effect runs
  // on mount too, and a walk into or out of a section mounts a new rail with
  // the level's first row focused. Without the guard, More would take it.
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

  // The sheet exists only at phone width; the control that closes it is not
  // rendered above the breakpoint. Widening the window while it is open would
  // otherwise leave the page locked and inert with nothing on screen to release
  // it. The width is already subscribed to above, so this reads that answer
  // rather than opening a second media query of its own.
  useEffect(() => {
    if (sheetOpen && !phone) {
      setSheetOpen(false);
    }
  }, [sheetOpen, phone]);

  // A scrim that only looks blocking is the worst of both worlds: the page it
  // dims stays reachable by Tab and by a screen reader. `inert` on the content
  // column takes it out of the tab order, the accessibility tree and pointer
  // reach in one attribute. That is the guarantee <dialog> gives, and this nav
  // cannot become one without giving up its landmark. The body stops scrolling
  // for the same reason: a touch on the scrim should dismiss, not scroll.
  //
  // The skip link goes inert with it. It is a sibling of the nav, not a child
  // of the column. So Tab past the sheet's last row wrapped onto it, drew it
  // over the scrim, and pointed at an inert element. That is a control that
  // takes focus, shows itself, and does nothing.
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

  // A destination the phone bar hides behind More: there More is the current
  // tab, and claims what the row it stands in for would (`currentClaim`).
  const sheetRow = NAV.find((item) => item.screen === level.shown.activeId);
  const inSheet = sheetRow ? !MOBILE_PRIMARY.has(sheetRow.screen) : false;
  const moreClaim = inSheet ? currentClaim(level.shown) : undefined;

  // The agent is app-level chrome: there is one of it, and it belongs to the
  // whole session rather than to any destination. A drilled-in level is
  // navigation inside one destination. An agent under it would read as that
  // destination's own: re-parented under a sub-level, and a second one the
  // moment a reader walks back out.
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
      >
        <div className="railhead">
          <BrandBlock />
        </div>
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
          // At phone width the agent stands in the middle of the bar, not at
          // the foot of a column that is not there. It sits in the row stream,
          // so a thumb and a Tab key read the bar in the same order. One Core
          // either way: the foot below renders only above the breakpoint.
          centre={phone ? <AgentRail route={route} bar={nav} /> : undefined}
        />
        {/* Phone-width only: expands the bar into a sheet carrying every
          destination. Hidden by CSS on the desktop sidebar.
          It carries the active state for every destination it hides, so the
          closed bar always shows where you are. The four tabs cannot, because
          those routes' own rows are display:none at this width. */}
        <button
          type="button"
          ref={more}
          className={inSheet ? "railmore active" : "railmore"}
          // Open, this control closes the sheet, so it says so in the name as
          // well as in the glyph. A control whose name stays the same while its
          // job changes is the one a screen reader gets wrong.
          aria-label={sheetOpen ? t("shell.closeMenu") : t("shell.more")}
          aria-expanded={sheetOpen}
          // The state has to reach a screen reader as well as the eye: the
          // hidden row is out of the accessibility tree at this width. Dropped
          // once the sheet is open, because the real row is then visible and
          // carries it. Two elements claiming the current page is worse than
          // none.
          aria-current={sheetOpen ? undefined : moreClaim}
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
