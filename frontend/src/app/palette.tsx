import { Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Badge,
  EmptyState,
  Kbd,
  PendingBody,
  SearchField,
} from "../design-system/atoms";
import { useBackdropDismiss } from "../design-system/backdrop";
import { Callout } from "../design-system/callout";
import { liveDialogs, useDialogFocus } from "../design-system/dialogfocus";
import { usePresence } from "../design-system/presence";
import { SCHEDULED_SCREEN } from "../features/mail/scheduledqueue";
import { useVisibleSettingsPages } from "../features/settings/settingsnav";
import { settingsPageKeywords } from "../features/settings/settingssearch";
import { holdsEveryWord, searchWords } from "../format/collate";
import { useLocale, useT } from "../i18n";
import { useCompanyContextShown } from "../lib/companycontextreach";
import { isImeCommit } from "../lib/imecommit";
import { navigate, type Route } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { openAsk, type UrlParams } from "../lib/urlstate";
import {
  CUSTOM_SCREEN,
  customPaletteScreens,
  resolveCustomLabel,
} from "./custom";
import { CREATE_ID, NAV } from "./nav";
import { PaletteAnswer, usePaletteAsk } from "./paletteanswer";
import {
  settingsCommandId,
  useConfigurationCommand,
} from "./paletteconfiguration";
import { PaletteRow } from "./paletterow";
import { SEARCH_PENDING_DELAY_MS, useSearchCommands } from "./palettesearch";

// ⌘K command palette (B-EP09.5, AC-shell-3..7). The command set carries a type
// tag (screen / action / record); record entries are fed by the search seam
// once the data layer lands — the ranking mechanics are already here.

export type Command = {
  id: string;
  label: string;
  subtitle?: string;
  // Extra terms the row answers to but does not display. A nav label is a
  // presentation choice and the domain vocabulary outlives it: Pipeline is still
  // the place deals live, Decisions is still the inbox, and someone typing the
  // older word must not be told the screen does not exist.
  keywords?: readonly string[];
  type: "screen" | "action" | "record";
  // The heading the row is drawn under, already translated. A record hit names
  // its kind here rather than in a badge, so a list holding a company and three
  // emails reads as two groups and not as four rows saying "Record".
  group?: string;
  // A message, drawn as the product's one citation of an email (subject and
  // date) in place of the label and second line. The date arrives formatted:
  // the caller owns the reader's timezone.
  cite?: Readonly<{ subject: string | null | undefined; occurredAt: string }>;
  // A contact's or a company's mark, drawn in place of the row's glyph: the
  // same chip the record wears everywhere else, keyed on its id.
  mark?: Readonly<{ identity: string; name: string; logo?: string | null }>;
  // Where the row goes. Absent on a row that opens something OVER the page
  // instead of leaving it — asking does that, and a route it never follows
  // would be a claim about where the reader ends up that is simply untrue.
  route?: Route;
  // How the page at `route` opens narrowed: the query half of the address.
  dials?: UrlParams;
};

/** The commands whose label, subtitle and keywords hold every query word. */
export function matchingCommands(
  commands: readonly Command[],
  query: string,
): readonly Command[] {
  const words = searchWords(query);
  if (words.length === 0) {
    return commands;
  }
  return commands.filter((command) =>
    holdsEveryWord(words, [
      command.label,
      command.subtitle ?? "",
      ...(command.keywords ?? []),
    ]),
  );
}

export function useBuiltinCommands(): Command[] {
  const t = useT();
  const { locale } = useLocale();
  // The same table the settings rail walks, not a second opinion about it: a
  // palette reading its own list offers a page the rail no longer lists.
  const visible = useVisibleSettingsPages();
  // Company profile draws its website card only where `companyContextShown`
  // says, so only that reader finds the page by the card's words.
  const seesCompanyContext = useCompanyContextShown();
  return useMemo(() => {
    const screens: Command[] = NAV.map((item) => ({
      id: `screen:${item.screen}`,
      label: t(item.labelKey),
      // The route id is the screen's stable English name and doubles as its
      // alias, so a destination stays findable under it in every locale. The
      // row's own aliases (app/nav.ts) ride alongside it, which is what keeps a
      // word a reader already learned pointing at the row that carries it.
      keywords: [item.screen, ...(item.aliases ?? [])],
      type: "screen",
      route: { screen: item.screen },
    }));
    // A fork's own screens (app/custom.ts), asked for by name here rather than
    // inherited from a rail entry: the rail is where things LIVE and the palette
    // is what you can DO, and a fork screen can honestly want one without the
    // other — a surface opened from a record has no rail row and is still worth
    // finding by typing its name. So this reads `palette`, not `nav`.
    //
    // Empty upstream, like every other arm of this seam.
    const forkScreens: Command[] = customPaletteScreens().map((screen) => ({
      id: `screen:${CUSTOM_SCREEN}/${screen.key}`,
      label: resolveCustomLabel(screen.palette.label, locale, t),
      keywords: [screen.key],
      type: "screen",
      route: { screen: CUSTOM_SCREEN, id: screen.key },
    }));
    const actions: Command[] = [
      {
        id: "action:new-deal",
        label: t("action.newDeal"),
        type: "action",
        route: { screen: "deals", id: CREATE_ID },
      },
      {
        id: "action:booking",
        label: t("action.booking"),
        type: "action",
        route: { screen: "book" },
      },
    ];
    // Every settings entry, derived from the register rather than hand-listed.
    // No settings entry is a rail row, so nothing else reaches them: a
    // hand-listed set omits entries only a reader who already knows the
    // shelving can open, where deriving brings a new tab here for free.
    //
    // Gated on the SAME predicate the settings level uses, because that level
    // answers an entry the principal may not open with the access boundary — so
    // an ungated command would be a shortcut to a refusal.
    // Only the admin half has a predicate; the `you` half is every reader's.
    const settingsScreens: Command[] = visible.map((page) => ({
      id: settingsCommandId(page.id),
      label: t(`settings.tab.${page.id}`),
      keywords: [
        page.id,
        ...settingsPageKeywords(page.id, t, { seesCompanyContext }),
        ...(page.id === "company" && seesCompanyContext
          ? [t("settings.companyRefresh")]
          : []),
      ],
      type: "screen",
      route: settingsHref(page.id),
    }));
    // The scheduled queue, which is off the rail deliberately — a queue of one
    // contact's own unsent mail is not an eleventh destination (pagemeta.ts says
    // so) — and was therefore reachable only by typing the address. The
    // composer that queued a message is one door; this is the other, for the
    // rep who closed that toast an hour ago and now wants the message back.
    //
    // Beside the settings shortcuts rather than among the rail screens above,
    // because those are built FROM the rail: a command for a screen the rail
    // does not carry belongs where the other off-rail ones already are.
    const offRailScreens: Command[] = [
      {
        id: `screen:${SCHEDULED_SCREEN}`,
        label: t("nav.scheduled"),
        // The words a rep would type for it, which are not the words on the
        // page: they think of the CONTROL they used, not the destination. So
        // the alias is that control's own label — already translated, so it is
        // the right words in each language rather than English prose a German
        // or Vietnamese reader would never type.
        //
        // The route id rides along beside it for the reason every rail command
        // above carries one: a stable English name that survives a relabel.
        keywords: [SCHEDULED_SCREEN, t("compose.scheduleSend")],
        type: "screen",
        route: { screen: SCHEDULED_SCREEN },
      },
    ];
    return [
      ...screens,
      ...forkScreens,
      ...actions,
      ...offRailScreens,
      ...settingsScreens,
    ];
  }, [t, visible, locale, seesCompanyContext]);
}

export function CommandPalette({
  open,
  onClose,
  commands,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  commands: Command[];
}>) {
  const t = useT();
  const [query, setQuery] = useState("");
  // The row the reader moved to with the arrows; null until they do, which
  // leaves Enter to defaultSelection.
  const [picked, setPicked] = useState<number | null>(null);
  const panel = useRef<HTMLDivElement>(null);
  const overlay = useRef<HTMLDivElement>(null);

  // The palette draws its own chrome and borrows the two contracts every dialog
  // in this product keeps. This is the second of them: the exit animation needs
  // the box to still be on the page to play on, and React would have taken it
  // off on the render that closed it. The keyframes are the centred dialog's
  // own, admitted to those rules by class (atoms.css).
  const { mounted, state } = usePresence({ open, element: overlay });

  // And the first: Escape from anywhere inside, Tab kept in, and focus returned
  // to whatever opened this when it closes. The palette had none of the three —
  // Escape belonged to the search input, so it did nothing from a result row,
  // and Shift+Tab left for the page behind on the first press. Keyed on `open`
  // and not on `mounted`, so the reader gets their place back the moment they
  // dismiss rather than at the end of an animation.
  useDialogFocus({ open, onClose, container: panel });

  // AC-shell-3: opening CLEARS the input. Focus is the hook's — the input is
  // this dialog's first tab stop, so it lands there either way, and two owners
  // of one focus move is how they come to disagree.
  useEffect(() => {
    if (open) {
      setQuery("");
      setPicked(null);
    }
  }, [open]);

  const matched = useMemo(
    () => matchingCommands(commands, query),
    [commands, query],
  );
  const setting = useConfigurationCommand(query, commands, matched);
  const filtered = setting ? [...matched, setting] : matched;

  // RS-1: live record hits from /search, plus a "see all" row that lands
  // on the full results screen. Row order: builtin matches, then see-all,
  // then records, so the row Enter takes on a half-typed name is one arrow
  // above the hits it summarises.
  const search = useSearchCommands(query);
  const seeAll: Command | null = query.trim()
    ? {
        id: "search:all",
        label: t("palette.seeAll", { query: query.trim() }),
        type: "action",
        route: { screen: "search", id: encodeURIComponent(query.trim()) },
      }
    : null;

  const handbook = usePaletteAsk(query, open, filtered.length);
  const rows = [
    ...(handbook.row ? [handbook.row] : []),
    ...filtered,
    ...(seeAll ? [seeAll] : []),
    ...search.commands,
  ];
  const clamp = (index: number) =>
    Math.max(0, Math.min(index, rows.length - 1));
  const selected = picked ?? defaultSelection(rows, filtered.length, query);

  const run = (command: Command) => {
    if (command.id === "ask-ai") {
      handbook.ask();
      return;
    }
    onClose();
    if (command.route) {
      navigate(command.route, command.dials);
    }
  };

  const leaving = state === "closing";
  const backdrop = useBackdropDismiss(onClose, leaving);

  if (!mounted) {
    return null;
  }

  return (
    // The two a11y suppressions this element carried are gone rather than kept:
    // `aria-hidden` below takes the overlay out of the accessibility tree while
    // it leaves, and the rules that wanted a keyboard handler beside the
    // backdrop click no longer fire on it. Escape is still the keyboard path,
    // and it is `useDialogFocus`'s.
    <div // NOSONAR: backdrop dismiss only; keyboard path (Esc) is handled on the input inside
      className="overlay palette-overlay"
      data-state={state}
      // Painted and nothing else while it leaves: `inert` takes it out of the
      // tab order and out of hit testing, `aria-hidden` out of the
      // accessibility tree — an inert node keeps its role, so without the
      // second one a palette on its way out is still a dialog to a reader.
      inert={leaving}
      aria-hidden={leaving || undefined}
      ref={overlay}
      {...backdrop}
    >
      <div
        className="palette"
        // NOSONAR: styled overlay palette, not a native modal; conditional mount and layout don't map cleanly to <dialog>
        role="dialog"
        aria-modal="true"
        aria-label={t("palette.aria")}
        ref={panel}
        // Focusable so the trap has somewhere to put focus on a query that
        // matched nothing — the list is then empty and the input is the only
        // other stop.
        tabIndex={-1}
      >
        <div className="palette-input">
          <SearchField
            flush
            value={query}
            placeholder={t("palette.placeholder")}
            aria-label={t("palette.aria")}
            onChange={(event) => {
              setQuery(event.target.value);
              setPicked(null);
            }}
            onKeyDown={(event) => {
              // No Escape arm here: `useDialogFocus` answers it for the whole
              // dialog. It was wired to this input alone, so Escape pressed
              // while focus sat on a result row — which is where the arrow keys
              // put a reader — did nothing at all.
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setPicked(clamp(selected + 1));
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setPicked(clamp(selected - 1));
              } else if (
                event.key === "Enter" &&
                !isImeCommit(event) &&
                rows[selected]
              ) {
                run(rows[selected]);
              }
            }}
          />
          {/* The key's own name, not copy: it is what is printed on the cap a
              reader is looking at, in every locale, the way the ⌘/Ctrl caps
              beside the search box are. */}
          <Kbd>{"esc"}</Kbd>
        </div>
        {handbook.shown ? (
          <PaletteAnswer
            pending={handbook.mutation.isPending}
            error={handbook.mutation.error}
            result={handbook.mutation.data}
            onLeave={onClose}
          />
        ) : null}
        <div className="palette-list">
          {/* A failed search says so and keeps the builtin commands beside it.
              Neither an EmptyState nor `danger`: the list is not empty, and the
              one thing a reader must not conclude is that there is nothing. */}
          {search.failed && (
            <Callout
              tone="warning"
              kind="outcome"
              title={t("palette.searchFailedTitle")}
            >
              {search.failure}
            </Callout>
          )}
          {/* Held back until the wait is real (SEARCH_PENDING_DELAY_MS): a bar
              that flashed on every keystroke would report work already done.
              The clock runs from the moment the bar appears and is NOT restarted
              when one query replaces another under it — a reader typing through
              a slow search should see one steady bar, not one that blinks out
              and returns per letter. */}
          {/* The drawer that asks one set of the reader's choosing, outside the
              rows so Enter on a screen name never opens it. It carries what is
              typed into the box rather than asking it. A question the box can
              read as one gets its own "Ask Margince" row below instead. */}
          <button
            type="button"
            className="palette-ask t-body"
            onClick={() => {
              onClose();
              openAsk(query.trim());
            }}
          >
            <Sparkles aria-hidden />
            <span className="label">{t("corpusAsk.title")}</span>
            <Badge tone="ai">{t("palette.typeAction")}</Badge>
          </button>
          {search.pending && (
            <PendingBody
              label={t("palette.searching")}
              lines={1}
              delayMs={SEARCH_PENDING_DELAY_MS}
            />
          )}
          {rows.length === 0 && !search.pending && !search.failed && (
            <EmptyState>{t("palette.empty")}</EmptyState>
          )}
          {rows.map((command, index) => (
            <PaletteRow
              key={command.id}
              command={command}
              previous={rows[index - 1]}
              selected={index === selected}
              onRun={run}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

// Where Enter lands before the reader picks a row. A destination the words
// matched keeps it: typing "pipeline" means going to Deals. A record found by
// search does not, because half a name is still a search. Enter then takes the
// first row, unless the words name a record whole. That row is the ask row for
// a question, and see-all for anything else.
function defaultSelection(
  rows: readonly Command[],
  destinations: number,
  query: string,
): number {
  if (destinations > 0) {
    return 0;
  }
  const needle = query.trim().toLowerCase();
  const named = rows.findIndex(
    (row) => row.type === "record" && row.label.toLowerCase() === needle,
  );
  return Math.max(named, 0);
}

// Global ⌘K / Ctrl+K binding (AC-shell-3).
// Both chords work, but an affordance may only advertise ONE and it has to be
// the one the reader's keyboard has: a Windows user told to press ⌘K is told to
// press a key that is not there. Pure in its argument, so the call site passes
// `navigator.platform` and this stays testable without stubbing it.
/**
 * The chord as its KEYS, because the one surface that draws it draws a cap per
 * key (app/topbar.tsx).
 *
 * Keys rather than a joined string, and no regex: the top bar used to split a
 * "⌘K" label with a lookbehind, which is a parse-time SyntaxError on an engine
 * without lookbehind (Safari before 16.4) — a blank app rather than a
 * plain-looking shortcut.
 */
export function paletteHotkeyCaps(platform: string): readonly string[] {
  return /mac|iphone|ipad|ipod/i.test(platform) ? ["⌘", "K"] : ["Ctrl", "K"];
}

export function usePaletteHotkey(
  open: boolean,
  setOpen: (open: boolean) => void,
) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        // An open dialog makes the rest of the app unreachable, the palette with
        // it; asked only while closed, so the palette's own box never blocks it.
        if (open || liveDialogs().length === 0) {
          setOpen(!open);
        }
      }
    };
    globalThis.addEventListener("keydown", onKey);
    return () => globalThis.removeEventListener("keydown", onKey);
  }, [open, setOpen]);
}
