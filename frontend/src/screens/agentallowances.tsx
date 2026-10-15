// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../app/unsaved";
import { Button, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { QueryGate } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { StatCard } from "../design-system/statcard";
import { StatStrip } from "../design-system/statstrip";
import { boundsText } from "../format/bounds";
import { formatDateTime, formatNumber } from "../format/format";
import { formatInterval } from "../format/interval";
import { UTC_ZONE } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { problemMessageOf } from "../lib/problem";
import { useInstallationSettings } from "../lib/uploadlimit";
import { useResetLine } from "./agentusage";
import { useUpdateInstallationSettings } from "./installation-settings";

type Allowances = components["schemas"]["AgentAllowanceSettings"];
type Counts =
  | "agent_writes_per_window"
  | "agent_egress_per_window"
  | "agent_calls_per_window";
type Draft = Readonly<Record<Counts | "agent_window_hours", string>>;

// The bounds the API refuses past, mirrored so a value out of range is refused
// in the box. Keyed by the wire property, which is how
// backend/gates/settingbounds_test.go holds each to the contract.
const AGENT_ALLOWANCE_BOUNDS = {
  agent_writes_per_window: { min: 10, max: 5_000 },
  agent_egress_per_window: { min: 1, max: 200 },
  agent_calls_per_window: { min: 50, max: 20_000 },
  agent_window_hours: { min: 1, max: 168 },
} as const satisfies Record<
  Counts | "agent_window_hours",
  { min: number; max: number }
>;

const COUNTS = [
  { property: "agent_writes_per_window", copy: "writes", release: true },
  { property: "agent_egress_per_window", copy: "egress", release: false },
  { property: "agent_calls_per_window", copy: "calls", release: false },
] as const satisfies readonly {
  property: Counts;
  copy: string;
  release: boolean;
}[];

// The lengths an admin picks from. The one already saved is added when it is
// not among them, so a value set through the API still shows and keeps.
const WINDOW_PRESETS = [1, 6, 12, 24, 168] as const;

// What one passport may do in each window: the values at rest, and an edit
// form behind a button. A count binds within a minute, a window length at the
// next reset.
export function AgentAllowancesCard() {
  const t = useT();
  const canRead = useCan("installation_settings", "read");
  const canManage = useCanWrite("installation_settings", "update");
  const query = useInstallationSettings(canRead);
  if (!canRead) {
    return null;
  }
  const title = t("agentAllowances.title");
  return (
    <Panel title={title}>
      <PanelBody className="form-stack">
        <PanelIntro>{t("agentAllowances.sub")}</PanelIntro>
        {!canManage && <PanelIntro>{t("operations.adminOnly")}</PanelIntro>}
      </PanelBody>
      <QueryGate query={query} pendingLabel={title}>
        {(settings) => {
          // A server older than this screen answers without them.
          if (settings.agent_allowances == null) {
            return null;
          }
          return (
            <AllowancesBody
              allowances={settings.agent_allowances}
              canManage={canManage}
            />
          );
        }}
      </QueryGate>
    </Panel>
  );
}

function AllowancesBody({
  allowances,
  canManage,
}: Readonly<{ allowances: Allowances; canManage: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft | null>(null);
  useUnsavedGuard(draft !== null);
  // A new allowance is the limit every usage bar is drawn against.
  const update = useUpdateInstallationSettings(() => {
    setDraft(null);
    for (const key of [["passports"], ["agent-usage"]]) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  });
  const resetLine = useResetLine();
  return (
    <>
      {draft === null ? (
        <AllowanceReadout allowances={allowances} locale={locale} />
      ) : (
        <AllowanceForm
          allowances={allowances}
          draft={draft}
          locale={locale}
          busy={update.isPending}
          onDraft={setDraft}
          onSave={(patch) => update.mutate(patch)}
          onCancel={() => {
            setDraft(null);
            update.reset();
          }}
        />
      )}
      <PanelBody className="form-stack">
        <p className="t-caption">
          {t("agentAllowances.windowLine", {
            interval: formatInterval(
              allowances.agent_window_hours * 3_600,
              locale,
            ),
          })}
          {" · "}
          {resetLine(allowances.agent_window_resets_at)}
        </p>
        <PendingWindow allowances={allowances} />
        {draft === null && <DefaultsNote allowances={allowances} />}
        {draft === null && canManage && (
          <div className="card-actions">
            <Button
              onClick={() => {
                update.reset();
                setDraft(draftOf(allowances));
              }}
            >
              {t("agentAllowances.edit")}
            </Button>
          </div>
        )}
        {update.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("operations.updateFailed")}
          >
            {problemMessageOf(update.error, t)}
          </Callout>
        )}
        <p className="t-caption">
          {t("agentAllowances.burst", {
            calls: formatNumber(allowances.mcp_calls_per_minute, locale),
            streams: formatNumber(
              allowances.mcp_stream_opens_per_minute,
              locale,
            ),
          })}
        </p>
      </PanelBody>
    </>
  );
}

// The window field shows the length the admin chose, waiting or not. Choosing
// the length in force again is then an edit, and it cancels the waiting change.
function chosenWindow(allowances: Allowances): number {
  return allowances.agent_window_next_hours ?? allowances.agent_window_hours;
}

function draftOf(allowances: Allowances): Draft {
  return {
    agent_writes_per_window: String(allowances.agent_writes_per_window),
    agent_egress_per_window: String(allowances.agent_egress_per_window),
    agent_calls_per_window: String(allowances.agent_calls_per_window),
    agent_window_hours: String(chosenWindow(allowances)),
  };
}

function defaultsDraft(allowances: Allowances): Draft {
  const defaults = allowances.defaults;
  return {
    agent_writes_per_window: String(defaults.agent_writes_per_window),
    agent_egress_per_window: String(defaults.agent_egress_per_window),
    agent_calls_per_window: String(defaults.agent_calls_per_window),
    agent_window_hours: String(defaults.agent_window_hours),
  };
}

function AllowanceReadout({
  allowances,
  locale,
}: Readonly<{ allowances: Allowances; locale: Locale }>) {
  const t = useT();
  return (
    <StatStrip testId="allowance-readout">
      {COUNTS.map((row) => (
        <StatCard
          key={row.property}
          label={t(`agentAllowances.${row.copy}.label`)}
          value={formatNumber(allowances[row.property], locale)}
          detail={t(
            row.release
              ? "agentAllowances.note.release"
              : "agentAllowances.note.stop",
          )}
        />
      ))}
    </StatStrip>
  );
}

// Says in words whether any value is off its default, so a reader of the plain
// numbers knows if they are looking at what ships.
function DefaultsNote({ allowances }: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const atDefaults =
    COUNTS.every(
      (row) => allowances[row.property] === allowances.defaults[row.property],
    ) && chosenWindow(allowances) === allowances.defaults.agent_window_hours;
  return (
    <Callout
      tone="info"
      title={t(
        atDefaults
          ? "agentAllowances.state.defaults"
          : "agentAllowances.state.changed",
      )}
    >
      {t("agentAllowances.state.body")}
    </Callout>
  );
}

function AllowanceForm({
  allowances,
  draft,
  locale,
  busy,
  onDraft,
  onSave,
  onCancel,
}: Readonly<{
  allowances: Allowances;
  draft: Draft;
  locale: Locale;
  busy: boolean;
  onDraft: (next: Draft) => void;
  onSave: (
    patch: Partial<Record<Counts | "agent_window_hours", number>>,
  ) => void;
  onCancel: () => void;
}>) {
  const t = useT();
  const parsed = parseDraft(draft);
  const patch = changesOf(allowances, parsed);
  const changed = Object.keys(patch).length > 0;
  return (
    <>
      <SettingList bleed="settings">
        {COUNTS.map((row) => {
          const refused = parsed[row.property] === null;
          return (
            <SettingRow
              key={row.property}
              label={t(`agentAllowances.${row.copy}.label`)}
              description={t(`agentAllowances.${row.copy}.help`)}
              control={(control) => (
                <div className="settingrow-measure numbersetting">
                  <TextInput
                    {...control}
                    className="t-num"
                    data-testid={`allowance-${row.property}`}
                    inputMode="numeric"
                    aria-describedby={
                      [control["aria-describedby"], `${control.id}-note`]
                        .filter(Boolean)
                        .join(" ") || undefined
                    }
                    aria-invalid={refused ? true : undefined}
                    value={draft[row.property]}
                    disabled={busy}
                    onChange={(e) =>
                      onDraft({ ...draft, [row.property]: e.target.value })
                    }
                  />
                  <span id={`${control.id}-note`} className="t-caption">
                    {refused
                      ? t(
                          "operations.refusal.count",
                          boundsText(
                            AGENT_ALLOWANCE_BOUNDS[row.property],
                            locale,
                          ),
                        )
                      : t("agentAllowances.default", {
                          value: formatNumber(
                            allowances.defaults[row.property],
                            locale,
                          ),
                        })}
                  </span>
                </div>
              )}
            />
          );
        })}
        <SettingRow
          label={t("agentAllowances.window.label")}
          description={t("agentAllowances.window.help")}
          control={(control) => (
            <Select
              {...control}
              value={draft.agent_window_hours}
              disabled={busy}
              options={windowOptions(allowances, locale)}
              onChange={(value) =>
                onDraft({ ...draft, agent_window_hours: value })
              }
              testId="allowance-agent_window_hours"
            />
          )}
        />
      </SettingList>
      <PanelBody className="form-stack">
        {changed && (
          <Diff allowances={allowances} patch={patch} locale={locale} />
        )}
        <div className="form-actions">
          <Button
            variant="primary"
            disabled={!changed || hasRefusal(parsed)}
            pending={busy}
            onClick={() => onSave(patch)}
          >
            {t("agentAllowances.save")}
          </Button>
          <Button disabled={busy} onClick={onCancel}>
            {t("agentAllowances.cancel")}
          </Button>
          <Button
            disabled={busy}
            onClick={() => onDraft(defaultsDraft(allowances))}
          >
            {t("agentAllowances.useDefaults")}
          </Button>
        </div>
      </PanelBody>
    </>
  );
}

type Parsed = Readonly<Record<Counts | "agent_window_hours", number | null>>;

// Digits only, inside the bounds the API refuses past: `Number` would also
// read "1e3" and " -5 " as numbers.
function parseDraft(draft: Draft): Parsed {
  const read = (property: keyof Draft): number | null => {
    const text = draft[property].trim();
    if (!/^\d+$/.test(text)) {
      return null;
    }
    const { min, max } = AGENT_ALLOWANCE_BOUNDS[property];
    const value = Number(text);
    return value >= min && value <= max ? value : null;
  };
  return {
    agent_writes_per_window: read("agent_writes_per_window"),
    agent_egress_per_window: read("agent_egress_per_window"),
    agent_calls_per_window: read("agent_calls_per_window"),
    agent_window_hours: read("agent_window_hours"),
  };
}

function hasRefusal(parsed: Parsed): boolean {
  return Object.values(parsed).some((value) => value === null);
}

// Only what moved is sent, so an untouched count leaves no audit entry.
function changesOf(
  allowances: Allowances,
  parsed: Parsed,
): Partial<Record<Counts | "agent_window_hours", number>> {
  const patch: Partial<Record<Counts | "agent_window_hours", number>> = {};
  for (const row of COUNTS) {
    const next = parsed[row.property];
    if (next !== null && next !== allowances[row.property]) {
      patch[row.property] = next;
    }
  }
  const hours = parsed.agent_window_hours;
  if (hours !== null && hours !== chosenWindow(allowances)) {
    patch.agent_window_hours = hours;
  }
  return patch;
}

function windowOptions(allowances: Allowances, locale: Locale) {
  const hours = new Set<number>([...WINDOW_PRESETS, chosenWindow(allowances)]);
  return [...hours]
    .sort((a, b) => a - b)
    .map((value) => ({
      value: String(value),
      label: formatInterval(value * 3_600, locale),
    }));
}

// What a save would do, before it does it. A count binds within a minute; a
// window length waits for the next reset, and the line says so.
function Diff({
  allowances,
  patch,
  locale,
}: Readonly<{
  allowances: Allowances;
  patch: Partial<Record<Counts | "agent_window_hours", number>>;
  locale: Locale;
}>) {
  const t = useT();
  const lines = COUNTS.flatMap((row) => {
    const next = patch[row.property];
    return next === undefined
      ? []
      : [
          t("agentAllowances.diff.count", {
            label: t(`agentAllowances.${row.copy}.label`),
            from: formatNumber(allowances[row.property], locale),
            to: formatNumber(next, locale),
          }),
        ];
  });
  const hours = patch.agent_window_hours;
  return (
    <Callout tone="warning" title={t("agentAllowances.diff.title")}>
      {lines.length > 0 && <p>{lines.join(" · ")}</p>}
      {lines.length > 0 && <p>{t("agentAllowances.diff.counts")}</p>}
      {hours !== undefined && (
        <p>
          {t("agentAllowances.diff.window", {
            from: formatInterval(chosenWindow(allowances) * 3_600, locale),
            to: formatInterval(hours * 3_600, locale),
          })}
        </p>
      )}
    </Callout>
  );
}

// A saved window length that has not taken over yet. Said out loud, because
// the readout still shows the length in force and a reader would think the
// save did nothing.
function PendingWindow({ allowances }: Readonly<{ allowances: Allowances }>) {
  const t = useT();
  const { locale } = useLocale();
  const hours = allowances.agent_window_next_hours;
  const from = allowances.agent_window_next_starts_at;
  if (hours == null || from == null) {
    return null;
  }
  return (
    <Callout tone="info" title={t("agentAllowances.pending.title")}>
      {t("agentAllowances.pending.body", {
        interval: formatInterval(hours * 3_600, locale),
        date: formatDateTime(from, locale, UTC_ZONE),
      })}
    </Callout>
  );
}
