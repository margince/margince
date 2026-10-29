import { Badge } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { useSettingsSection } from "../features/settings/settingsnav";
import { useLocale, useT } from "../i18n";
import { EXTENSION_SCREEN, findExtension } from "../lib/extensions";
import type { Route } from "../lib/router";
import { usePhoneViewport } from "../lib/viewport";
import { entryLabel, NAV, type NavSection, recordKindOf } from "./nav";
import {
  headsItself,
  PAGE_SUB_KEYS,
  routeTitle,
  sectionHead,
} from "./pagemeta";
import { SectionSwitcher } from "./sectionswitcher";
import { TopBar } from "./topbar";

/**
 * The chrome on a settings route, naming the tab the reader opened.
 *
 * Both halves read the same section the rail does. The hook derives it from
 * the capability cache the rail already warmed. So the trail, the heading and
 * the sidebar cannot disagree about which tab is current.
 */
export function SettingsTopBar({
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

export function SettingsPageTitle(
  props: Readonly<{ route: Route; actionSlot: SlotRef }>,
) {
  const section = useSettingsSection(props.route);
  return <PageTitle {...props} section={section} />;
}

type SlotRef = (slot: HTMLElement | null) => void;

/**
 * The page's own name, standing in the content column above what it names.
 *
 * It sits inside the scroller and scrolls away with the page. The heading
 * belongs to the document, while the top bar belongs to the chrome. The top
 * bar says where you are and stays; this says what the page is and goes.
 *
 * No page is named twice. A record's surface prints its own identity block,
 * and a composed unit names its own screen, so this yields to both. A second
 * name at heading level would leave a screen reader with two page titles.
 */
export function PageTitle({
  route,
  section,
  actionSlot,
}: Readonly<{
  route: Route;
  section?: NavSection;
  /** Takes the box the page's own verb (PageHeadAction) is drawn into. */
  actionSlot?: SlotRef;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const phone = usePhoneViewport();

  const navItem = NAV.find((item) => item.screen === route.screen);
  const inSection = sectionHead(section, route);
  // On a screen that publishes a level, the page is the entry the reader
  // opened, not the section they opened it from. The trail in the top bar and
  // the sidebar's level name the section. Printing it here named the section
  // twice and the surface never. A settings page read "Settings" above a
  // heading reading "Settings", with the audit log under both.
  const entryTitle = inSection
    ? entryLabel(inSection.entry, locale, t)
    : undefined;
  const title = entryTitle ?? routeTitle(route, navItem?.labelKey, locale, t);
  // A record kind, and only then. An id segment that names no record is a
  // screen's own state (the settings tab, for one), and the page is still the
  // screen. Printing that slug as the page's name gave Settings an h1 reading
  // "privacy".
  const recordNamesPage = recordKindOf(route) !== undefined;
  // Conditioned on the descriptor resolving, not on the screen slug alone. A
  // unit route is kept out of both the `NAV` rail and `OFF_RAIL_TITLE_KEYS`.
  // So `resolveTitle` falls through to `shell.unknownPage`. A unit this
  // installation did not compose is an unknown page, and its screen says so in
  // words. That case keeps the heading rather than yielding to a surface that
  // will not name itself either.
  const unitNamesPage =
    route.screen === EXTENSION_SCREEN && findExtension(route.id) !== null;
  // A page that heads itself. Brief greets the reader by name in its own h1.
  // The shell printing the nav label above it named the page twice at heading
  // level: an outline with two top-level headings. The branches above exist to
  // prevent that for records and units.
  //
  // Asked of the route, through pagemeta's one predicate. A focused filter page
  // heads itself where the library under the same screen does not. The
  // predicate is the one place to read which pages do this.
  const selfHeaded = headsItself(route);
  // Read only on the branch that prints an h1. A surface naming itself gets no
  // subtitle from here either, or the page would describe a heading it is not
  // showing. The entry's own line comes first. A section is many pages behind
  // one screen, and the screen-keyed table carries only a sentence true of all
  // of them. The table remains for screens that are one page.
  const subKey = inSection?.entry.subKey ?? PAGE_SUB_KEYS[route.screen];
  // Whose state the page changes. Only a settings entry carries one, and only
  // then: on every other screen the answer is the record in front of you.
  const scopeKey = inSection?.entry.scopeKey;

  if (recordNamesPage || unitNamesPage || selfHeaded) {
    return null;
  }

  // At phone width a section's page is its switcher. The control names the
  // page and opens the others, so it stands as the heading rather than under a
  // heading repeating it. The page is still named once at heading level. The
  // trail above it is the only other place the name appears, the same
  // arithmetic as every other route.
  const switcher = phone && inSection && (
    <SectionSwitcher section={inSection.section} entry={inSection.entry} />
  );

  return (
    <div className="pagetitle">
      <div className="pagetitle-text">
        {/* The heading is named by the page even when a control is all it
            contains. Name-from-content would otherwise reach into the
            switcher's own `aria-label` (the section name plus a "change
            section" instruction). That would put an instruction in the
            document's heading list. The control keeps that name; the heading
            states the page. */}
        {/* One row for heading and scope: as a sibling the badge would read as
            a second line of the title. */}
        <div className="pagetitle-head">
          <Heading
            size="xlarge"
            className={switcher ? "t-display pageswitchhead" : "t-display"}
            aria-label={switcher ? title : undefined}
          >
            {switcher || title}
          </Heading>
          {/* Beside the heading, not in it: a heading carrying the scope would
              read "Capture rules Company" in every outline and screen reader.
              Neutral: company-wide is the ordinary case, not a warning. */}
          {scopeKey && (
            <Badge>
              {/* The mixed page says something different from the others.
                  It says it has no single answer for who it affects, and each
                  setting states its own. "Who this page affects: Mixed" would
                  be a non-sentence. */}
              <span className="sr-only">
                {scopeKey === "settings.scope.mixed"
                  ? t("settings.scopeAriaMixed")
                  : t("settings.scopeAria", { scope: t(scopeKey) })}
              </span>
              <span aria-hidden="true">{t(scopeKey)}</span>
            </Badge>
          )}
        </div>
        {subKey && <p className="pagesub t-sub">{t(subKey)}</p>}
      </div>
      <div className="pagehead-action" ref={actionSlot} />
    </div>
  );
}
