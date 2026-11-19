import type { LucideIcon } from "lucide-react";
import {
  type ComponentProps,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Button, Kbd, PendingBody, SearchField } from "../design-system/atoms";
import { useBackdropDismiss } from "../design-system/backdrop";
import { Callout } from "../design-system/callout";
import { useDialogFocus } from "../design-system/dialogfocus";
import { Eyebrow } from "../design-system/eyebrow";
import { usePresence } from "../design-system/presence";
import type { StatusKind } from "../design-system/statusbadge";
import { holdsEveryWord, searchWords } from "../format/collate";
import { type Translator, useT } from "../i18n";
import { isImeCommit } from "../lib/imecommit";
import { navigate, type Route } from "../lib/router";
import type { CoreSearchHitType, SearchHitGroup } from "../lib/searchkinds";
import { openAsk, type UrlParams } from "../lib/urlstate";
import { PaletteAnswer, usePaletteAsk } from "./paletteanswer";
import {
  PaletteAskDocuments,
  PaletteEmpty,
  PaletteFilter,
  PaletteKeys,
} from "./palettechrome";
import { useBuiltinCommands } from "./palettecommands";
import { useConfigurationCommand } from "./paletteconfiguration";
import type { PaletteMode } from "./palettehotkey";
import { recentCommand, usePaletteRecents } from "./paletterecents";
import { type RecentEntry, recentIdentity } from "./paletterecents.storage";
import { PaletteRow } from "./paletterow";
import { SEARCH_PENDING_DELAY_MS, useSearchCommands } from "./palettesearch";

// ⌘K command palette. Each command is a screen, setting, action or record;
// record rows come from the search seam.

export type Command = {
  id: string;
  label: string;
  subtitle?: string;
  // Extra terms the row answers to but does not display. A nav label is a
  // presentation choice and the domain vocabulary outlives it: Pipeline is still
  // the place deals live, Decisions is still the inbox, and someone typing the
  // older word must not be told the screen does not exist.
  keywords?: readonly string[];
  // A settings page is a "setting": several share a list screen's name, and the
  // badge is what tells the two rows apart.
  type: "screen" | "setting" | "action" | "record";
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
  // The row's glyph where it has no mark: a screen's own nav icon.
  icon?: LucideIcon;
  // The kind of record a hit is, which picks its glyph.
  kind?: SearchHitGroup;
  // Where the record stands, drawn on the right: a status, or a quiet fact.
  status?: Readonly<{ kind: StatusKind; label: string }>;
  aside?: string;
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

const CREATE_COMMAND_PREFIX = "action:new-";

/** A command that makes a new record, which `C` lists where a screen has no form of its own. */
export function isCreateCommand(command: Command): boolean {
  return command.id.startsWith(CREATE_COMMAND_PREFIX);
}

// The search answered the words with no record, and no command matches them.
function foundNothing(
  search: Readonly<{ empty: boolean }>,
  matched: readonly Command[],
): boolean {
  return search.empty && matched.length === 0;
}

// The row that lands on the full results screen for the typed words. A results
// page that would say "No matches" is no row to offer.
function seeAllCommand(
  searched: string,
  t: Translator,
  nothingFound: boolean,
): Command | null {
  const words = searched.trim();
  if (!words || nothingFound) return null;
  return {
    id: "search:all",
    executedQuery: words,
    label: t("palette.seeAll", { query: words }),
    type: "action",
    route: { screen: "search", id: encodeURIComponent(words) },
  };
}

function builtinRows(
  kind: CoreSearchHitType | null,
  matched: readonly Command[],
  setting: Command | null,
): readonly Command[] {
  if (kind) return [];
  return setting ? [...matched, setting] : matched;
}

function present<T>(row: T | null | undefined): T[] {
  return row ? [row] : [];
}

export function CommandPalette({
  open,
  onClose,
  commands: offered,
  mode = "all",
}: Readonly<{
  open: boolean;
  onClose: () => void;
  commands: Command[];
  mode?: PaletteMode;
}>) {
  const t = useT();
  const [query, setQuery] = useState("");
  // The row the reader moved to with the arrows; null until they do, which
  // leaves Enter to defaultSelection.
  const [picked, setPicked] = useState<string | null>(null);
  // The one type of record the list is narrowed to; null lists every type.
  const [kind, setKind] = useState<CoreSearchHitType | null>(null);
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
  useDialogFocus({ open, onClose, container: panel, overlay });

  // AC-shell-3: opening CLEARS the input. Focus is the hook's — the input is
  // this dialog's first tab stop, so it lands there either way, and two owners
  // of one focus move is how they come to disagree.
  useEffect(() => {
    if (open) {
      setQuery("");
      setPicked(null);
      setKind(null);
    }
  }, [open]);

  const creates = mode === "create";
  const commands = useMemo(
    () => (creates ? offered.filter(isCreateCommand) : offered),
    [creates, offered],
  );
  // Create mode answers "make a record", so a typed word narrows those and asks nothing else.
  const searched = creates ? "" : query;
  const matched = useMemo(
    () => matchingCommands(commands, query),
    [commands, query],
  );
  const setting = useConfigurationCommand(searched, commands, matched);
  // Narrowed to one type, the list holds only records of it.
  const filtered = builtinRows(kind, matched, setting);

  // RS-1: live record hits from /search, plus a "see all" row that lands
  // on the full results screen. Row order: builtin matches, then see-all,
  // then records, so the row Enter takes on a half-typed name is one arrow
  // above the hits it summarises.
  // The unnarrowed answer counts every type; a narrowed one fills the list.
  const every = useSearchCommands(searched);
  const narrowed = useSearchCommands(kind ? searched : "", kind);
  const search = kind ? narrowed : every;
  const listsRecents = !creates && !query.trim();
  const recents = usePaletteRecents(open, listsRecents);
  const recentCommands = recents.entries.map((entry) =>
    recentCommand(entry, t, recents.activating === recentIdentity(entry)),
  );
  const nothingFound = foundNothing(search, filtered);
  const seeAll = seeAllCommand(searched, t, nothingFound);

  const handbook = usePaletteAsk(searched, open, filtered.length);
  const askGroup = t("palette.askGroup");
  const rows = [
    ...recentCommands,
    ...filtered,
    ...present(seeAll),
    ...search.commands,
    ...present(handbook.row).map((row) => ({ ...row, group: askGroup })),
  ];
  const clamp = (index: number) =>
    Math.max(0, Math.min(index, rows.length - 1));
  const selected = paletteSelection(
    rows,
    filtered.length,
    query,
    picked,
    listsRecents && recents.pending,
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
    <div // NOSONAR: backdrop dismiss only; keyboard path (Esc) is handled by useDialogFocus
      className="overlay palette-overlay"
      data-state={state}
      // Painted and nothing else while it leaves: `inert` stops Tab and hit
      // testing now; `useDialogFocus` hides it from readers once focus is out.
      inert={leaving}
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
        {!creates && Object.keys(every.counts).length > 0 && (
          <PaletteFilter
            value={kind}
            counts={every.counts}
            floors={every.floors}
            onChange={(next) => {
              setKind(next);
              setPicked(null);
            }}
          />
        )}
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
          {search.pending && (
            <PendingBody
              label={t("palette.searching")}
              lines={1}
              delayMs={SEARCH_PENDING_DELAY_MS}
            />
          )}
          <PaletteEmpty
            query={searched.trim()}
            nothingFound={nothingFound}
            listed={rows.length}
            search={search}
          />
          {listsRecents && recents.hasStored && (
            <Eyebrow
              as="h2"
              size="xxsmall"
              className="palette-group"
              action={
                <Button
                  variant="link"
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
          {listsRecents && recents.pending && (
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
          {!creates && (
            <PaletteAskDocuments
              heading={rows.at(-1)?.group === askGroup ? null : askGroup}
              onAsk={() => {
                onClose();
                openAsk(query.trim());
              }}
            />
          )}
        </div>
        <PaletteKeys />
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
  if (named >= 0) return named;
  // Then a question to ask, else the first row.
  for (const preferred of DEFAULT_ROWS) {
    const index = rows.findIndex(preferred);
    if (index >= 0) return index;
  }
  return 0;
}

const DEFAULT_ROWS: readonly ((row: Command) => boolean)[] = [
  (row) => row.id === "ask-ai",
];

export function BuiltinCommandPalette(
  props: Omit<ComponentProps<typeof CommandPalette>, "commands">,
) {
  return <CommandPalette {...props} commands={useBuiltinCommands()} />;
}
