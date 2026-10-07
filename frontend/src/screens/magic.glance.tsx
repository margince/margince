// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The receipt at a glance: the largest kinds of work done in the window, each
// one reading, so "what happened" is answered before a single line is read.

import { StatCard } from "../design-system/statcard";
import { StatStrip } from "../design-system/statstrip";
import { floorFigure } from "../format/figure";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { type MagicSentenceKey, magicByKey } from "./magic.keys";
import { fillsPage, type MagicLine } from "./magic.queries";

// Which reading each sentence counts toward, for EVERY sentence the receipt
// can send, so a new one is a compile error here until somebody decides. Null
// is a sentence that never lands in the done lane, and several sentences share
// one reading where a reader would not tell the work apart.
const READING_OF: Readonly<Record<MagicSentenceKey, MessageKey | null>> = {
  "magic.action.advance_stage": "magic.glance.dealsMoved",
  "magic.action.promote": "magic.glance.leadsPromoted",
  "magic.action.update": "magic.glance.recordsUpdated",
  "magic.action.assign": "magic.glance.ownersChanged",
  "magic.action.activity_relink": "magic.glance.activitiesRelinked",
  "magic.action.send_email": "magic.glance.messagesSent",
  "magic.action.schedule": "magic.glance.meetingsBooked",
  "magic.action.disqualify": "magic.glance.leadsDisqualified",
  "magic.action.mail_filed": "magic.glance.emailsFiled",
  "magic.action.company_profile_read": "magic.glance.profilesRead",
  "magic.action.fields_changed": "magic.glance.recordsUpdated",
  "magic.action.retention_lead_anonymize": "magic.glance.retention",
  "magic.action.retention_lead_archive": "magic.glance.retention",
  "magic.action.retention_contact_anonymize": "magic.glance.retention",
  "magic.action.retention_contact_erase": "magic.glance.retention",
  "magic.action.retention_activity_archive": "magic.glance.retention",
  "magic.action.retention_activity_erase": "magic.glance.retention",
  "magic.action.retention_deal_archive": "magic.glance.retention",
  "magic.action.create_contact": "magic.glance.recordsCreated",
  "magic.action.create_company": "magic.glance.recordsCreated",
  "magic.action.create_deal": "magic.glance.recordsCreated",
  "magic.action.create_lead": "magic.glance.recordsCreated",
  "magic.action.create_project": "magic.glance.recordsCreated",
  "magic.action.create_activity": "magic.glance.recordsCreated",
  "magic.action.archive_contact": "magic.glance.recordsArchived",
  "magic.action.archive_company": "magic.glance.recordsArchived",
  "magic.action.archive_deal": "magic.glance.recordsArchived",
  "magic.action.archive_lead": "magic.glance.recordsArchived",
  "magic.action.archive_project": "magic.glance.recordsArchived",
  "magic.action.archive_activity": "magic.glance.recordsArchived",
  "magic.action.automation_troubled": null,
  "magic.action.approval_coldstart": null,
  "magic.action.approval_send_email": null,
  "magic.action.approval_advance_deal": null,
  "magic.action.approval_promote_lead": null,
  "magic.action.approval_overnight": null,
  "magic.action.approval_commitment_task": null,
  "magic.action.approval_capture_counterparty": null,
  "magic.action.approval_pending": null,
  "magic.action.capture_reauth_required": null,
  "magic.action.capture_connection_error": null,
  "magic.action.capture_sync_failing": null,
  "magic.action.capture_backfill_failed": null,
};

const READINGS: ReadonlyMap<string, MessageKey | null> = new Map(
  Object.entries(READING_OF),
);

// Three, as the strip reads across Home's main column: a fourth folds the row
// onto a second line, and the rest are one press away in the list below.
const GLANCE_SLOTS = 3;

type GlanceReading = Readonly<{
  label: MessageKey;
  total: number;
  // A line counted into it was cut short, so the total is only a floor.
  floor: boolean;
  // The job that did most of it, named the way the line below names it.
  actor: MagicLine["actor"] | null;
}>;

type ActorShare = {
  actor: MagicLine["actor"];
  records: number;
  floor: boolean;
};

type Tally = {
  total: number;
  floor: boolean;
  actors: Map<string, ActorShare>;
};

/**
 * The done lane folded into readings: records per kind of work, largest first.
 *
 * Records rather than lines, because one line may stand for 1,200 filed emails
 * and a tile reading "1" would say the opposite of what happened. A sentence
 * this build predates counts toward no reading; its line still stands below.
 */
function glanceReadings(done: readonly MagicLine[]): readonly GlanceReading[] {
  const byLabel = new Map<MessageKey, Tally>();
  for (const line of done) {
    const label = READINGS.get(line.summary.key);
    if (!label) {
      continue;
    }
    const count = line.count ?? 1;
    const floor = line.count_is_floor === true;
    const tally = byLabel.get(label) ?? {
      total: 0,
      floor: false,
      actors: new Map(),
    };
    tally.total += count;
    tally.floor ||= floor;
    const who = `${line.actor.type}:${line.actor.id}`;
    const share = tally.actors.get(who) ?? {
      actor: line.actor,
      records: 0,
      floor: false,
    };
    share.records += count;
    share.floor ||= floor;
    tally.actors.set(who, share);
    byLabel.set(label, tally);
  }
  // A lane that filled its page may hold more of any kind beyond it: every
  // total is then a floor, and no job can be said to have done the most.
  const full = fillsPage("done", done);
  return [...byLabel.entries()]
    .map(([label, tally]) => ({
      label,
      total: tally.total,
      floor: tally.floor || full,
      actor: full ? null : leadActor(tally.actors),
    }))
    .sort((a, b) => b.total - a.total)
    .slice(0, GLANCE_SLOTS);
}

// The job with the most records in a reading. None where another job's count
// was cut short, since its true share could pass the leader's.
function leadActor(
  actors: ReadonlyMap<string, ActorShare>,
): MagicLine["actor"] | null {
  const [lead, ...rest] = [...actors.values()].sort(
    (a, b) => b.records - a.records,
  );
  if (!lead || rest.some((share) => share.floor)) {
    return null;
  }
  return lead.actor;
}

export function MagicGlance({
  done,
}: Readonly<{ done: readonly MagicLine[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const readings = glanceReadings(done);
  if (readings.length === 0) {
    return null;
  }
  return (
    <StatStrip label={t("magic.glance.label")} className="magic-glance">
      {readings.map((reading) => {
        const by = reading.actor?.label
          ? magicByKey(reading.actor.label.key)
          : null;
        return (
          <StatCard
            key={reading.label}
            label={t(reading.label)}
            value={floorFigure(
              formatNumber(reading.total, locale),
              reading.floor,
              reading.total,
            )}
            detail={by ? t(by, reading.actor?.label?.values) : undefined}
          />
        );
      })}
    </StatStrip>
  );
}
