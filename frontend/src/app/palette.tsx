import { Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Badge,
  Button,
  EmptyState,
  Kbd,
  PendingBody,
  SearchField,
} from "../design-system/atoms";
import { useBackdropDismiss } from "../design-system/backdrop";
import { Callout } from "../design-system/callout";
import { liveDialogs, useDialogFocus } from "../design-system/dialogfocus";
import { Eyebrow } from "../design-system/eyebrow";
import { usePresence } from "../design-system/presence";
import { holdsEveryWord, searchWords } from "../format/collate";
import { useT } from "../i18n";
import { ENTITY } from "../lib/entity";
import { isImeCommit } from "../lib/imecommit";
import { navigate, type Route } from "../lib/router";
import { SEARCH_HIT_KIND_KEY, searchHitHasCard } from "../lib/searchkinds";
import { openAsk, type UrlParams } from "../lib/urlstate";
import { modifierCap } from "./hotkeys";
import { PaletteAnswer, usePaletteAsk } from "./paletteanswer";
import { useConfigurationCommand } from "./paletteconfiguration";
import { usePaletteRecents } from "./paletterecents";
import { type RecentEntry, recentIdentity } from "./paletterecents.storage";
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
  // Execution provenance keeps replay separate from recording a new search.
  executedQuery?: string;
  searchedRecord?: Extract<RecentEntry, { kind: "record" }>;
  recent?: RecentEntry;
  pending?: boolean;
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

export { useBuiltinCommands } from "./palettecommands";

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
  const [picked, setPicked] = useState<string | null>(null);
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
  const recents = usePaletteRecents(open, !query.trim());
  const recentCommands: Command[] = recents.entries.map((entry) => ({
    id: `recent:${recentIdentity(entry)}`,
    label: entry.kind === "query" ? entry.text : entry.label,
    subtitle:
      entry.kind === "record"
        ? t(SEARCH_HIT_KIND_KEY[entry.entity_type])
        : undefined,
    type: entry.kind === "query" ? "action" : "record",
    group: t("palette.recent"),
    recent: entry,
    pending: recents.activating === recentIdentity(entry),
    route:
      entry.kind === "record"
        ? ENTITY[entry.entity_type].route(entry.id)
        : undefined,
    mark:
      entry.kind === "record" && searchHitHasCard(entry.entity_type)
        ? { identity: entry.id, name: entry.label, logo: entry.logo }
        : undefined,
  }));
  const seeAll: Command | null = query.trim()
    ? {
        id: "search:all",
        executedQuery: query.trim(),
        label: t("palette.seeAll", { query: query.trim() }),
        type: "action",
        route: { screen: "search", id: encodeURIComponent(query.trim()) },
      }
    : null;

  const handbook = usePaletteAsk(query, open, filtered.length);
  const rows = [
    ...recentCommands,
    ...(handbook.row ? [handbook.row] : []),
    ...filtered,
    ...(seeAll ? [seeAll] : []),
    ...search.commands,
  ];
  const clamp = (index: number) =>
    Math.max(0, Math.min(index, rows.length - 1));
  const selected = paletteSelection(
    rows,
    filtered.length,
    query,
    picked,
    recents.pending,
  );
  const focusSearch = () => {
    setPicked(null);
    panel.current
      ?.querySelector<HTMLInputElement>('input[type="search"]')
      ?.focus();
  };

  const run = (command: Command) => {
    if (command.pending) return;
    if (command.id === "ask-ai") {
      handbook.ask();
      return;
    }
    if (command.recent?.kind === "query") {
      setQuery(command.recent.text);
      focusSearch();
      return;
    }
    if (command.recent?.kind === "record") {
      void recents.openRecord(
        command.recent,
        () => {
          onClose();
          if (command.route) navigate(command.route);
        },
        focusSearch,
      );
      return;
    }
    if (command.executedQuery)
      void recents.remember(command.executedQuery, command.searchedRecord);
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
              recents.cancel();
              setQuery(event.target.value);
              setPicked(null);
            }}
            onKeyDown={(event) => {
              // No Escape arm here: `useDialogFocus` answers it for the whole dialog.
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setPicked(rows[clamp(selected + 1)]?.id ?? null);
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setPicked(rows[clamp(selected - 1)]?.id ?? null);
              } else if (
                event.key === "Enter" &&
                !isImeCommit(event) &&
                rows[selected]
              ) {
                // Closing returns focus to the opener, and Enter's keypress would
                // then activate it: the top-bar search button reopens the palette.
                event.preventDefault();
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
          {!query.trim() && recents.hasStored && (
            <Eyebrow
              as="h2"
              size="xxsmall"
              className="palette-group"
              action={
                <Button
                  variant="ghost"
                  onClick={() => {
                    recents.clear();
                    focusSearch();
                  }}
                >
                  {t("palette.clearRecent")}
                </Button>
              }
            >
              {t("palette.recent")}
            </Eyebrow>
          )}
          {!query.trim() && recents.pending && (
            <PendingBody label={t("common.loading")} lines={1} />
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

// Selection follows identity so validation cannot move an arrow-picked target.
// Empty-box Enter waits until the recent list has been checked.
function paletteSelection(
  rows: readonly Command[],
  destinations: number,
  query: string,
  picked: string | null,
  pending: boolean,
): number {
  if (picked !== null) return rows.findIndex((row) => row.id === picked);
  if (!query.trim() && pending) return -1;
  if (destinations > 0) {
    return 0;
  }
  const needle = query.trim().toLowerCase();
  const named = rows.findIndex(
    (row) => row.type === "record" && row.label.toLowerCase() === needle,
  );
  return Math.max(named, 0);
}

const PALETTE_KEY = "k";

/**
 * The ⌘K / Ctrl K chord as one cap per key. Both modifiers work, but a surface
 * shows only the one the reader's keyboard has. Pure in `platform`, so a test
 * needs no stub.
 */
export function paletteHotkeyCaps(platform: string): readonly string[] {
  return [modifierCap(platform), PALETTE_KEY.toUpperCase()];
}

export function usePaletteHotkey(
  open: boolean,
  setOpen: (open: boolean) => void,
) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (
        (event.metaKey || event.ctrlKey) &&
        event.key.toLowerCase() === PALETTE_KEY
      ) {
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
