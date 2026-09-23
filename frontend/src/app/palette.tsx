import { CornerDownLeft, Sparkles } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  Badge,
  EmptyState,
  Kbd,
  PendingBody,
  SearchField,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useDialogFocus } from "../design-system/dialogfocus";
import { usePresence } from "../design-system/presence";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { SCHEDULED_SCREEN } from "../screens/scheduledsends";
import type { SettingsPageId } from "../screens/settingscatalog";
import { useVisibleSettingsPages } from "../screens/settingsnav";
import { settingsHref } from "../screens/settingsrouting";
import {
  CUSTOM_SCREEN,
  customPaletteScreens,
  resolveCustomLabel,
} from "./custom";
import { CREATE_ID, NAV } from "./nav";
import { SEARCH_PENDING_DELAY_MS, useSearchCommands } from "./palettesearch";
import { navigate, type Route } from "./router";
import { openAsk } from "./urlstate";

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
  // Where the row goes. Absent on a row that opens something OVER the page
  // instead of leaving it — asking does that, and a route it never follows
  // would be a claim about where the reader ends up that is simply untrue.
  route?: Route;
};

// The words a settings entry answers to beyond its own label. A reader types the
// THING they are looking for — "webhook", "products", "password" — and almost
// never the name of the page it was filed under, which is a shelving decision
// they were not present for.
//
// Three of these carry words the product no longer prints anywhere: three
// screens of their own collapsed into the data-model page and the automations
// editor into the AI page, and somebody who learned "custom fields" or
// "automations" must not be told the product no longer has one.
const SETTINGS_ALIASES: Readonly<
  Partial<Record<SettingsPageId, readonly string[]>>
> = {
  account: ["password", "profile", "language", "theme"],
  voice: ["tone", "writing style"],
  agents: ["passport", "api key", "token"],
  connections: ["oauth", "mailbox", "calendar"],
  "capture-activity": ["capture log", "trace"],
  company: ["general", "currency", "workspace", "fx"],
  authentication: ["sign-in", "sso", "oauth app", "login"],
  members: ["users", "contacts", "roster", "invite"],
  teams: ["team"],
  seats: ["license", "billing", "plan", "subscription"],
  pipelines: ["stages", "deal stages"],
  leads: ["lead sources", "disqualify"],
  fields: ["custom-fields", "data model", "schema"],
  tags: ["labels", "vocabulary"],
  products: ["price list", "rate card", "offer-templates"],
  capture: ["email capture", "inbox"],
  integrations: ["webhook", "api"],
  knowledge: ["handbook", "corpus", "documents"],
  import: ["csv", "upload", "migration"],
  models: ["routing", "providers", "keys", "embeddings"],
  automations: ["rules", "triggers"],
  usage: ["spend", "cost", "budget", "tokens"],
  "model-calls": ["logs", "trace", "calls"],
  privacy: ["gdpr", "consent", "retention", "erasure"],
  audit: ["trail", "log", "history"],
  "system-health": ["jobs", "health", "reindex", "maintenance"],
  extensions: ["units", "plugins"],
  reset: ["danger", "wipe", "delete everything"],
};

export function useBuiltinCommands(): Command[] {
  const t = useT();
  const { locale } = useLocale();
  // The same table the settings rail walks, not a second opinion about it: a
  // palette reading its own list offers a page the rail no longer lists.
  const visible = useVisibleSettingsPages();
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
        id: "action:read-company",
        label: t("action.readCompany"),
        type: "action",
        route: { screen: "onboarding", id: "company" },
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
    // falls back to Account for an entry the principal may not open — so an
    // ungated command would be a shortcut that silently goes somewhere else.
    // Only the admin half has a predicate; the `you` half is every reader's.
    const settingsScreens: Command[] = visible.map((page) => ({
      id: `screen:settings-${page.id}`,
      label: t(`settings.tab.${page.id}`),
      keywords: [page.id, ...(SETTINGS_ALIASES[page.id] ?? [])],
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
  }, [t, visible, locale]);
}

const TYPE_KEY: Record<Command["type"], MessageKey> = {
  screen: "palette.typeScreen",
  action: "palette.typeAction",
  record: "palette.typeRecord",
};

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
  const [selected, setSelected] = useState(0);
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
      setSelected(0);
    }
  }, [open]);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return commands;
    }
    return commands.filter(
      (command) =>
        command.label.toLowerCase().includes(needle) ||
        (command.subtitle ?? "").toLowerCase().includes(needle) ||
        (command.keywords ?? []).some((word) =>
          word.toLowerCase().includes(needle),
        ),
    );
  }, [commands, query]);

  // RS-1: live record hits from /search, plus a "see all" row that lands
  // on the full results screen. Row order: builtin matches, then records,
  // then see-all, then the Ask-AI row last.
  const search = useSearchCommands(query);
  const seeAll: Command | null = query.trim()
    ? {
        id: "search:all",
        label: t("palette.seeAll", { query: query.trim() }),
        type: "action",
        route: { screen: "search", id: encodeURIComponent(query.trim()) },
      }
    : null;

  // Asking leads, always. The palette answers two different questions — where
  // do I go, and what does this company know — and only the first has a list of
  // destinations to scan. A reader who came to ASK had to type something and
  // then hunt past every screen whose name happened to match it, so the row sat
  // last on the one journey it exists for. It carries the query when there is
  // one and opens an empty box when there is not; either way it goes nowhere.
  const rows = [...filtered, ...search.commands, ...(seeAll ? [seeAll] : [])];
  const clamp = (index: number) =>
    Math.max(0, Math.min(index, rows.length - 1));

  const run = (command: Command) => {
    onClose();
    if (command.id === "ask-ai") {
      openAsk(query.trim());
      return;
    }
    if (command.route) {
      navigate(command.route);
    }
  };

  if (!mounted) {
    return null;
  }
  const leaving = state === "closing";

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
      onClick={(event) => {
        if (leaving) {
          return;
        }
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
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
              setSelected(0);
            }}
            onKeyDown={(event) => {
              // No Escape arm here: `useDialogFocus` answers it for the whole
              // dialog. It was wired to this input alone, so Escape pressed
              // while focus sat on a result row — which is where the arrow keys
              // put a reader — did nothing at all.
              if (event.key === "ArrowDown") {
                event.preventDefault();
                setSelected((index) => clamp(index + 1));
              } else if (event.key === "ArrowUp") {
                event.preventDefault();
                setSelected((index) => clamp(index - 1));
              } else if (event.key === "Enter" && rows[selected]) {
                run(rows[selected]);
              }
            }}
          />
          {/* The key's own name, not copy: it is what is printed on the cap a
              reader is looking at, in every locale, the way the ⌘/Ctrl caps
              beside the search box are. */}
          <Kbd>{"esc"}</Kbd>
        </div>
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
              {t("palette.searchFailed")}
            </Callout>
          )}
          {/* Held back until the wait is real (SEARCH_PENDING_DELAY_MS): a bar
              that flashed on every keystroke would report work already done.
              The clock runs from the moment the bar appears and is NOT restarted
              when one query replaces another under it — a reader typing through
              a slow search should see one steady bar, not one that blinks out
              and returns per letter. */}
          {/* Asking, always, and deliberately NOT one of the options below.
              The palette answers two questions — where do I go, and what does
              this company know — and only the first has a list to scan. Pinned
              into that list it took the first row, which is the row Enter
              presses, so a reader typing a screen name would have asked about
              it instead of going there. Here it is reachable on sight and by
              Tab, and it carries whatever is typed into the box rather than
              asking it: a question matched mid-word is one still being
              written. */}
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
            <button
              key={command.id}
              type="button"
              className={
                index === selected
                  ? "palette-row t-body selected"
                  : "palette-row t-body"
              }
              onClick={() => run(command)}
              ref={(element) => {
                if (index === selected) {
                  element?.scrollIntoView?.({ block: "nearest" });
                }
              }}
            >
              {command.id === "ask-ai" ? (
                <Sparkles aria-hidden />
              ) : (
                <CornerDownLeft aria-hidden />
              )}
              <span className="label">{command.label}</span>
              {command.subtitle && (
                <span className="sub t-caption">{command.subtitle}</span>
              )}
              <Badge>{t(TYPE_KEY[command.type])}</Badge>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
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

export function usePaletteHotkey(toggle: () => void) {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        toggle();
      }
    };
    globalThis.addEventListener("keydown", onKey);
    return () => globalThis.removeEventListener("keydown", onKey);
  }, [toggle]);
}
