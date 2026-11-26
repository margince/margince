import { usePageTitle } from "../components/auth-identity";
import { useEntityName } from "../components/entityref";
import { translate, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useOpenableApp } from "../lib/extensionapps";
import { EXTENSION_SCREEN, findExtension, unitTitle } from "../lib/extensions";
import { opensAFocusedFiltersPage } from "../lib/filtersaddress";
import { useReadsTeamProgress } from "../lib/progressreader";
import type { Route, Screen } from "../lib/router";
import { SETTINGS_SCREEN } from "../lib/settingsrouting";
import {
  entryLabel,
  NAV,
  type NavLevelEntry,
  type NavSection,
  navLabelKey,
  parentScreenOf,
  recordKindOf,
  reservedPageTitle,
} from "./nav";

// What the chrome knows about a page before the page renders: its name, its
// one-line description, and which entry of a section it is.

// The line under the page's name, for the screens whose name alone does not
// say what the page is for. It belongs to the page rather than to anything on
// it, which is why it lives beside the title keys: a screen that printed its
// own subtitle had to print its own title above it to hang it on, and the
// shell was already printing that title — the page then named itself twice.
//
/**
 * Screens that head THEMSELVES, so the shell prints no heading above them.
 *
 * Brief greets the reader by name in its own h1 ("Guten Morgen, Demo"). The
 * shell's nav label above it named the page a second time at heading level.
 * Two top-level headings is no outline at all, and it is the same defect the
 * shell already avoids for a record (whose name is its heading) and a unit.
 *
 * A screen earns a place here by rendering an h1 of its own, not by having a
 * heading somewhere on it. A screen whose own heading is an h2 still wants the
 * shell to name the page.
 */
// Brief greets the reader in its own h1. The tag page heads itself with the
// TAG'S NAME, which the shell cannot know: "Tag" above a pill spelling
// "Automation World 2026" names the page twice and names it worse both times —
// a reader arriving from a search hit wants the word they searched at the top.
//
// The list screens head themselves in the header of the table that IS the page
// (`ListSurface`'s `title`): a name printed above that card said one word the
// trail had already said, and spent a whole band of a phone's screen doing it —
// on a 390px viewport the search box and the filter row started below the fold.
// A screen here is a screen that passes `title`, and the two must move together
// or the page is named twice at heading level.
export const SELF_HEADED_SCREENS: ReadonlySet<string> = new Set([
  "home",
  "tags",
  "lists",
  "contacts",
  "companies",
  "leads",
  "deals",
  "projects",
  "partners",
]);

/**
 * Whether the page at this route prints its own h1, so the shell prints none.
 * Asked of the route rather than the screen: a focused Filters and views page
 * heads itself with what it holds, while the library under the same screen
 * is named by the shell. `filters` stays out of the set for that reason.
 */
export function headsItself(route: Route): boolean {
  return (
    SELF_HEADED_SCREENS.has(route.screen) || opensAFocusedFiltersPage(route)
  );
}

// Only a subtitle true of the WHOLE page qualifies. Copy that describes the
// current tab, filter or segment belongs beside that control, where it changes
// with it; the page heading cannot see those and would go stale.
export const PAGE_SUB_KEYS: Record<string, MessageKey> = {
  // The library's line. It shows nowhere else under the screen: a focused
  // filter page heads itself (headsItself), and the shell's subtitle goes
  // with the heading it hangs on.
  filters: "filters.subtitle",
  // A scheduled send holds unsent words and blind copies, and only the one who
  // scheduled it can read it; the page states that once.
  scheduled: "sched.sub",
};

// Off-rail screens (reached from Settings or from the surface that created the
// thing, not the NAV rail) carry their own title key. Every authenticated route
// resolves to real copy — a raw screen slug is never shown as a page title.
export const OFF_RAIL_TITLE_KEYS: Record<string, MessageKey> = {
  settings: "nav.settings",
  offers: "nav.offers",
  partners: "nav.partners",
  share: "nav.share",
  search: "nav.search",
  // Off the rail on purpose. The rail carries the product's destinations, and a
  // queue of one user's own unsent mail is not one of them; it is
  // reached from the composer that put a message in it and from Today, which is
  // where the same rep's other waiting work already lives.
  scheduled: "nav.scheduled",
  // Off the rail because a tag page is reached from a tag — a pill on a record,
  // a search hit, the vocabulary in Settings — and never from a destination
  // list. Absent from this map it fell through to `shell.unknownPage`, so the
  // page that lists what carries a word was headed "Not found" above the
  // results it had just found.
  tags: "nav.tags",
  lists: "lists.page",
  // Reached from the deals page and a deal's legal tab, by a reader who reviews.
  "legal-reviews": "legalReview.queueTitle",
};

export function resolveTitle(
  screen: string,
  labelKey: MessageKey | undefined,
  t: ReturnType<typeof useT>,
): string {
  if (labelKey) {
    return t(labelKey);
  }
  const offRailKey = OFF_RAIL_TITLE_KEYS[screen];
  // An address nobody in this app answers. The screen under it says so in
  // words; the heading must not print the slug the reader typed as though the
  // product had a page by that name.
  return offRailKey ? t(offRailKey) : t("shell.unknownPage");
}

/**
 * The name of a page, in the words the sidebar uses for it.
 *
 * For a screen that heads itself; the shell's `PageTitle` names every other
 * route. Not `t("nav.contacts")` at the call site: a rename of the rail's label
 * would leave that heading behind.
 *
 * The screen is a parameter rather than read from the route, so a story that
 * renders the surface outside the router still names it correctly.
 */
export function usePageName(screen: Screen): string {
  const t = useT();
  const readsTeam = useReadsTeamProgress();
  return resolveTitle(
    screen,
    navLabelKey(
      NAV.find((item) => item.screen === screen),
      readsTeam,
    ),
    t,
  );
}

// What a section contributes to the chrome: the entry the reader opened. The
// section's own `activeId` is its answer and comes first, fallbacks included; the
// route's segment stands in only for a caller that resolved nothing.
export function sectionHead(
  section: NavSection | undefined,
  route: Route,
): { section: NavSection; entry: NavLevelEntry } | undefined {
  if (!section || section.screen !== route.screen) {
    return undefined;
  }
  const activeId = section.activeId ?? route.id;
  for (const group of section.groups) {
    const entry = group.items.find((item) => item.id === activeId);
    if (entry) {
      return { section, entry };
    }
  }
  return undefined;
}

/** The route a record stands under, for a place that reads no record's name. */
export function withoutRecord(route: Route): Route {
  return recordKindOf(route) ? { screen: route.screen } : route;
}

/**
 * Where the page at `route` sits, the page first. A record or a reserved page
 * comes before its list, a settings entry before its section, and a unit
 * before Settings. An off-rail page comes before the destination it belongs
 * to. A record joins once its name is read, so no path shows an id.
 */
export function usePagePath(
  route: Route,
  section?: NavSection,
): [string, ...string[]] {
  const t = useT();
  const { locale } = useLocale();
  const navItem = NAV.find((item) => item.screen === route.screen);
  const readsTeam = useReadsTeamProgress();
  const screenTitle = resolveTitle(
    route.screen,
    navLabelKey(navItem, readsTeam),
    t,
  );
  // An app is named by the words its unit declared for its drawer entry.
  const app = useOpenableApp(route);
  const recordKind = recordKindOf(route);
  const recordId = recordKind && route.id;
  const { name } = useEntityName(recordKind ?? "contact", recordId);
  if (recordId) {
    return name ? [name, screenTitle] : [screenTitle];
  }
  if (app) {
    return [translate(locale, app.label)];
  }
  const reserved = reservedPageTitle(route);
  if (reserved) {
    return [t(reserved), screenTitle];
  }
  const unit =
    route.screen === EXTENSION_SCREEN ? findExtension(route.id) : null;
  if (unit) {
    return [
      unitTitle(unit, locale),
      resolveTitle(SETTINGS_SCREEN, undefined, t),
    ];
  }
  const inSection = sectionHead(section, route);
  if (inSection) {
    return [
      entryLabel(inSection.entry, locale, t),
      t(inSection.section.titleKey),
    ];
  }
  const parent = parentScreenOf(route);
  if (parent) {
    const parentItem = NAV.find((item) => item.screen === parent);
    return [
      screenTitle,
      resolveTitle(parent, navLabelKey(parentItem, readsTeam), t),
    ];
  }
  return [screenTitle];
}

// The browser tab reads the path from the page back to the product, so two
// tabs and two history entries name two pages.
export function useTabTitle(route: Route, section?: NavSection) {
  const t = useT();
  const path = usePagePath(route, section);
  usePageTitle([...path, t("shell.logoAria")].join(" · "));
}
