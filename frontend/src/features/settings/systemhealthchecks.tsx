// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  Activity,
  ArchiveRestore,
  Bot,
  Inbox,
  type LucideIcon,
  Mail,
  RefreshCw,
  Search,
  Sparkles,
} from "lucide-react";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import { FactList } from "../../design-system/factlist";
import { OffsiteLink } from "../../design-system/offsitelink";
import { Panel, PanelBody } from "../../design-system/panel";
import { ServiceList, ServiceRow } from "../../design-system/servicerow";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { navigate, type Route } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import {
  healthAvailability,
  healthCodes,
  healthSourceLabels,
} from "./systemhealth.copy";
import { healthTone } from "./systemhealthstatus";

type Source = components["schemas"]["HealthSource"];
type Finding = components["schemas"]["HealthFinding"];

const RESTORE_GUIDE =
  "https://github.com/margince/margince/blob/main/docs/how-to/run-a-restore-drill.md";

/** What a check that was never set up says, and the one step that sets it up. */
type SetupGuide = Readonly<{
  happened: MessageKey;
  matters: MessageKey;
  todo: MessageKey;
  action: MessageKey;
  to: Readonly<{ route: Route } | { href: string }>;
}>;

// Keyed by the server's presentation code: each one names a part of the
// installation that nobody has set up yet, rather than one that broke.
const SETUP_GUIDES: Readonly<Record<string, SetupGuide>> = {
  providers_not_configured: {
    happened: "health.setup.providersMissing.happened",
    matters: "health.setup.providersMissing.matters",
    todo: "health.setup.providersMissing.todo",
    action: "health.setup.providersMissing.action",
    to: { route: settingsHref("models") },
  },
  providers_unobserved: {
    happened: "health.setup.providersSilent.happened",
    matters: "health.setup.providersSilent.matters",
    todo: "health.setup.providersSilent.todo",
    action: "health.setup.providersSilent.action",
    to: { route: settingsHref("models") },
  },
  search_not_configured: {
    happened: "health.setup.search.happened",
    matters: "health.setup.search.matters",
    todo: "health.setup.search.todo",
    action: "health.setup.search.action",
    to: { route: settingsHref("models", "tiers") },
  },
  recovery_missing: {
    happened: "health.setup.recovery.happened",
    matters: "health.setup.recovery.matters",
    todo: "health.setup.recovery.todo",
    action: "health.setup.recovery.action",
    to: { href: RESTORE_GUIDE },
  },
};

const SOURCE_ICONS: Readonly<Record<string, LucideIcon>> = {
  jobs: Activity,
  capture: Mail,
  intake: Inbox,
  providers: Sparkles,
  search: Search,
  updates: RefreshCw,
  recovery: ArchiveRestore,
  automatic: Bot,
};

function guideFor(source: Source): SetupGuide | undefined {
  return Object.hasOwn(SETUP_GUIDES, source.code)
    ? SETUP_GUIDES[source.code]
    : undefined;
}

/** The checks the overview sorts into its groups, each in one group only. */
export type HealthGroups = Readonly<{
  setup: readonly Source[];
  waiting: readonly Source[];
  passing: readonly Source[];
}>;

/**
 * Sort the checks a reader may see. A check an open finding already explains
 * stays with that finding, so it is never told twice.
 */
export function healthGroups(
  sources: readonly Source[],
  findings: readonly Finding[],
): HealthGroups {
  const explained = new Set(
    findings
      .filter((finding) => finding.requires_action)
      .map((finding) => finding.source),
  );
  const open = sources.filter(
    (source) => source.availability !== "withheld" && !explained.has(source.id),
  );
  const passing = open.filter((source) => healthTone(source) === "success");
  const setup = open.filter(
    (source) => !passing.includes(source) && guideFor(source) !== undefined,
  );
  return {
    setup,
    waiting: open.filter(
      (source) => !passing.includes(source) && !setup.includes(source),
    ),
    passing,
  };
}

/** The three groups below the findings: not set up, waiting, passing. */
export function HealthChecks({ groups }: Readonly<{ groups: HealthGroups }>) {
  const t = useT();
  return (
    <>
      {groups.setup.length > 0 && (
        <Panel title={t("health.group.setup")}>
          <PanelBody>
            <ServiceList label={t("health.group.setup")}>
              {groups.setup.map((source) => (
                <SetupRow key={source.id} source={source} />
              ))}
            </ServiceList>
          </PanelBody>
        </Panel>
      )}
      {groups.waiting.length > 0 && (
        <CheckList title={t("health.group.waiting")} sources={groups.waiting} />
      )}
      {groups.passing.length > 0 && (
        <CheckList title={t("health.group.passing")} sources={groups.passing} />
      )}
    </>
  );
}

function SetupRow({ source }: Readonly<{ source: Source }>) {
  const t = useT();
  const guide = guideFor(source);
  if (guide === undefined) return null;
  const { to } = guide;
  return (
    <ServiceRow
      icon={SOURCE_ICONS[source.id] ?? Activity}
      name={t(healthSourceLabels[source.id] ?? "health.unknown")}
      status={{ tone: "default", label: t("systemHealth.notSetUp") }}
    >
      <FactList
        facts={[
          {
            key: "happened",
            term: t("health.whatHappened"),
            value: t(guide.happened),
          },
          {
            key: "matters",
            term: t("health.whyItMatters"),
            value: t(guide.matters),
          },
          { key: "todo", term: t("health.whatToDo"), value: t(guide.todo) },
        ]}
      />
      <ActionRow className="health-setup-action">
        {"route" in to ? (
          <Button onClick={() => navigate(to.route)}>{t(guide.action)}</Button>
        ) : (
          <OffsiteLink href={to.href}>{t(guide.action)}</OffsiteLink>
        )}
      </ActionRow>
    </ServiceRow>
  );
}

function CheckList({
  title,
  sources,
}: Readonly<{ title: string; sources: readonly Source[] }>) {
  const t = useT();
  return (
    <Panel title={title}>
      <PanelBody>
        <ServiceList label={title}>
          {sources.map((source) => (
            <ServiceRow
              key={source.id}
              icon={SOURCE_ICONS[source.id] ?? Activity}
              name={t(healthSourceLabels[source.id] ?? "health.unknown")}
              caption={t(healthCodes[source.code] ?? "health.unknown")}
              status={checkStatus(source, t)}
            />
          ))}
        </ServiceList>
      </PanelBody>
    </Panel>
  );
}

function checkStatus(source: Source, t: (key: MessageKey) => string) {
  const tone = healthTone(source);
  if (tone === "success")
    return { tone, label: t("health.checkPassing") } as const;
  return {
    tone: tone === "danger" || tone === "warning" ? tone : "default",
    label: t(healthAvailability[source.availability]),
  } as const;
}
