import { useQueries, useQuery } from "@tanstack/react-query";
import { CornerDownLeft, Sparkles } from "lucide-react";
import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";
import { EmptyState, PendingBody, SearchField } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useDialogFocus } from "../design-system/dialogfocus";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { SCHEDULED_SCREEN } from "../screens/scheduledsends";
import {
  SETTINGS_TABS,
  type SettingsTabId,
  settingsAddress,
  useSettingsEntryVisibility,
} from "../screens/settings";
import {
  CUSTOM_SCREEN,
  customPaletteScreens,
  resolveCustomLabel,
} from "./custom";
import { NAV } from "./nav";
import { navigate, type Route } from "./router";
import {
  SEARCH_HIT_KIND_KEY,
  type SearchHitType,
  searchHitRoute,
} from "./searchkinds";

// ⌘K command palette (B-EP09.5, AC-shell-3..7). The command set carries a
// type tag (screen / action / record); record entries are fed by the search
// seam once the data layer lands — the tagging and ranking mechanics are
// already here. The "Ask AI: …" run-as-NL row is always appended last.

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
  route: Route;
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
  Partial<Record<SettingsTabId, readonly string[]>>
> = {
  account: ["password", "profile", "language", "theme"],
  voice: ["tone", "writing style"],
  agents: ["passport", "api key", "token"],
  connections: ["oauth", "mailbox", "calendar"],
  "capture-activity": ["capture log", "trace"],
  general: ["company", "currency", "workspace"],
  users: ["roles", "permissions", "seats", "team"],
  integrations: ["webhook", "api", "overlay"],
  capture: ["email capture", "inbox"],
  "data-model": [
    "custom-fields",
    "products",
    "price list",
    "rate card",
    "offer-templates",
    "pipelines",
    "tags",
  ],
  ai: ["automations", "models", "routing", "embeddings"],
  knowledge: ["handbook", "corpus", "documents"],
  privacy: ["gdpr", "consent", "retention", "erasure"],
  license: ["billing", "plan", "subscription"],
  maintenance: ["reset", "jobs", "health"],
};

export function useBuiltinCommands(): Command[] {
  const t = useT();
  const { locale } = useLocale();
  // Without the company rollout probe: the palette offers no shortcut to General,
  // and that probe is a network read this hook would otherwise fire on every
  // screen (see useSettingsEntryVisibility).
  const visible = useSettingsEntryVisibility(false);
  return useMemo(() => {
    const screens: Command[] = NAV.map((item) => ({
      id: `screen:${item.screen}`,
      label: t(item.labelKey),
      // The route id is the screen's stable English name and doubles as its
      // alias, so a relabeled destination stays findable under both words in
      // both locales without a hand-kept synonym list.
      keywords: [item.screen],
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
        route: { screen: "deals", id: "new" },
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
    // Two of them used to be named here and the rest were left out on the
    // grounds that "the rail door beside them" reached them — which was never
    // true of settings: no settings entry is a rail row, so an entry this list
    // omits is an entry only a reader who already knows the shelving can open.
    // Deriving also means a tab added to the register arrives here, instead of
    // being the third one somebody notices is missing.
    //
    // Gated on the SAME predicate the settings level uses, because that level
    // falls back to Account for an entry the principal may not open — so an
    // ungated command would be a shortcut that silently goes somewhere else.
    // Only the admin half has a predicate; the `you` half is every reader's.
    const settingsScreens: Command[] = SETTINGS_TABS.filter(
      (entry) => entry.group !== "admin" || visible[entry.id],
    ).map((entry) => ({
      id: `screen:settings-${entry.id}`,
      label: t(`settings.tab.${entry.id}`),
      keywords: [entry.id, ...(SETTINGS_ALIASES[entry.id] ?? [])],
      type: "screen",
      route: settingsAddress(entry.id),
    }));
    // The scheduled queue, which is off the rail deliberately — a queue of one
    // person's own unsent mail is not an eleventh destination (pagemeta.ts says
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

// How long a palette search may take before the wait is worth reporting. Below
// this the answer is quicker than a keystroke and a placeholder would flash on
// every letter typed; above it, an unchanged list reads as a palette that has
// stopped listening.
const SEARCH_PENDING_DELAY_MS = 300;

// What the live search arm has to say: the rows it found, and whether it is
// still working or gave up. The two flags are returned rather than swallowed —
// the palette used to answer a failed search with an empty array, which is the
// same shape as "no matches" and told the reader the workspace holds nothing
// when the truth was that nobody had asked it.
type SearchArm = Readonly<{
  commands: Command[];
  pending: boolean;
  failed: boolean;
}>;

// Live record hits for the palette (RS-1): debounced via useDeferredValue
// rather than a timer (craft: no real-clock waits in the render path), and
// gated on a 2-char floor so single keystrokes don't fire a query per key.
function useSearchCommands(query: string): SearchArm {
  const t = useT();
  const deferred = useDeferredValue(query.trim());
  const enabled = deferred.length >= 2;
  const result = useQuery({
    queryKey: ["palette-search", deferred],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q: deferred, limit: 5 } },
      });
      if (error) {
        // Thrown rather than flattened to an empty list: react-query carries it
        // to `isError`, and the palette says the search failed instead of
        // reporting an empty workspace. The builtin commands keep working
        // beside it, which is the degradation that was wanted — losing the
        // sentence was not.
        throw new Error(t("palette.searchFailed"));
      }
      return data.data;
    },
  });
  // Every hit with somewhere to go. `searchHitRoute` is the one place that
  // knows where each kind lives, so a type the server learns to return is
  // routable here the moment it is routable anywhere — and an activity, which
  // has no page, drops out by answering null rather than by being named in a
  // second list that has to be kept in step.
  //
  // An EMAIL hit is findable and openable on the search SCREEN, which owns a
  // drawer. The palette owns no page and every Command must carry a route, so
  // it cannot open one; issue #3850 holds what that would take.
  const hits = (result.data ?? []).flatMap((hit) => {
    const route = searchHitRoute(hit.type as SearchHitType, hit.id);
    return route ? [{ hit, route }] : [];
  });
  const projectLines = useProjectHitLines(
    hits.filter(({ hit }) => hit.type === "project").map(({ hit }) => hit.id),
  );
  return {
    commands: hits.map(({ hit, route }) => ({
      id: `record:${hit.type}:${hit.id}`,
      label: hit.title ?? hit.id,
      // A project's secondary line is its key or its company, not the word
      // "project": a search hit for one carries no snippet, and two projects
      // called "Rollout" are told apart by the key a rep already types into
      // subject lines. Every other kind names the kind — TRANSLATED, because
      // this line used to print the wire word and showed a German reader
      // "organization" where the rest of the product says Firma.
      subtitle:
        hit.type === "project"
          ? (projectLines.get(hit.id) ??
            t(SEARCH_HIT_KIND_KEY[hit.type as SearchHitType]))
          : t(SEARCH_HIT_KIND_KEY[hit.type as SearchHitType]),
      type: "record" as const,
      route,
    })),
    // `isFetching` rather than `isPending`: a disabled query reports pending
    // forever, and the palette opens with an empty box every time.
    pending: enabled && result.isFetching,
    failed: enabled && result.isError,
  };
}

/**
 * The secondary line for each project hit: the key when the project has one,
 * else the company's name. At most five hits are on screen, so the reads are
 * per record and share the cache entries the project page and the company
 * reference already fill.
 */
function useProjectHitLines(projectIds: string[]): Map<string, string> {
  const projects = useQueries({
    queries: projectIds.map((id) => ({
      queryKey: ["project", id, "ref"],
      staleTime: 60_000,
      queryFn: async () => {
        const { data, error } = await api.GET("/projects/{id}", {
          params: { path: { id } },
        });
        if (error) {
          // A palette line that cannot be resolved falls back to the kind;
          // the hit itself still routes. The project page reports the
          // failure in full.
          return null;
        }
        return data;
      },
    })),
  });
  const companyIds = projects.flatMap((query) =>
    query.data && !query.data.key && query.data.organization_id
      ? [query.data.organization_id]
      : [],
  );
  const companies = useQueries({
    queries: companyIds.map((id) => ({
      // The same entry EntityRef fills for a company reference.
      queryKey: ["organization", "ref", id],
      staleTime: 60_000,
      queryFn: async () => {
        const { data, error } = await api.GET("/organizations/{id}", {
          params: { path: { id } },
        });
        if (error) {
          return null;
        }
        return data.display_name ?? null;
      },
    })),
  });
  const companyName = new Map(
    companyIds.map((id, index) => [id, companies[index]?.data ?? null]),
  );
  const lines = new Map<string, string>();
  projects.forEach((query, index) => {
    const project = query.data;
    if (!project) {
      return;
    }
    const line =
      project.key ??
      (project.organization_id
        ? companyName.get(project.organization_id)
        : null);
    if (line) {
      lines.set(projectIds[index], line);
    }
  });
  return lines;
}

const TYPE_KEY: Record<Command["type"], MessageKey> = {
  screen: "palette.typeScreen",
  action: "palette.typeAction",
  record: "palette.typeRecord",
};

export const ASK_QUERY_KEY = "margince.askQuery";

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

  // What every dialog in this product owes the keyboard, from the one place it
  // is spelled: Escape from anywhere inside, Tab kept in, and focus returned to
  // whatever opened this when it closes. The palette had none of the three —
  // Escape belonged to the search input, so it did nothing from a result row,
  // and Shift+Tab left for the page behind on the first press.
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

  // The run-as-NL row (AC-shell-4): appended last whenever there is a query.
  const askRow: Command | null = query.trim()
    ? {
        id: "ask-ai",
        label: t("palette.askAi", { query: query.trim() }),
        type: "action",
        route: { screen: "ai" },
      }
    : null;
  const rows = [
    ...filtered,
    ...search.commands,
    ...(seeAll ? [seeAll] : []),
    ...(askRow ? [askRow] : []),
  ];
  const clamp = (index: number) =>
    Math.max(0, Math.min(index, rows.length - 1));

  const run = (command: Command) => {
    if (command.id === "ask-ai") {
      // NOSONAR: persisted value is a trimmed plain string from a controlled input, consumed as text (never eval'd or rendered as HTML)
      sessionStorage.setItem(ASK_QUERY_KEY, query.trim());
    }
    onClose();
    navigate(command.route);
  };

  if (!open) {
    return null;
  }

  return (
    // biome-ignore lint/a11y/noStaticElementInteractions: backdrop dismiss; Esc is the keyboard path
    // biome-ignore lint/a11y/useKeyWithClickEvents: Esc handled on the input below
    <div // NOSONAR: backdrop dismiss only; keyboard path (Esc) is handled on the input inside
      className="overlay palette-overlay"
      onClick={(event) => {
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
          <span className="kbd">{"esc"}</span>
        </div>
        <div className="palette-list">
          {/* A failed search says so and keeps the builtin commands beside it.
              It is not an EmptyState: the list is not empty, and the one thing
              a reader must not conclude is that the workspace holds nothing. */}
          {search.failed && (
            <Callout tone="warn" live="status" className="palette-notice">
              {t("palette.searchFailed")}
            </Callout>
          )}
          {/* Held back until the wait is real (SEARCH_PENDING_DELAY_MS): a bar
              that flashed on every keystroke would report work already done. */}
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
                index === selected ? "palette-row selected" : "palette-row"
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
                <span className="sub">{command.subtitle}</span>
              )}
              <span className="type">{t(TYPE_KEY[command.type])}</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

// Global ⌘K / Ctrl+K binding (AC-shell-3).
// The palette answers to Meta+K and Ctrl+K both, but an affordance may only
// advertise one, and it has to be the one the reader's keyboard has: a Windows
// user told to press ⌘K is being told to press a key that is not there. Pure in
// its argument so the call site passes `navigator.platform` and this stays
// testable without stubbing the platform.
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
