// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { knownTriggerLabel } from "./automations.recipe";

type AutomationRun = components["schemas"]["AutomationRun"];
type Words = Readonly<{ did: MessageKey; queued: MessageKey }>;

// What a run did or queued, per action kind, with {target} for the record.
// automations.runwords.test.ts reads the kinds from the server's Go.
export const RUN_ACTION_WORDS: Readonly<Record<string, Words>> = {
  create_record: {
    did: "auto.runs.did.create_record",
    queued: "auto.runs.queued.create_record",
  },
  update_record: {
    did: "auto.runs.did.update_record",
    queued: "auto.runs.queued.update_record",
  },
  create_task: {
    did: "auto.runs.did.create_task",
    queued: "auto.runs.queued.create_task",
  },
  assign_owner: {
    did: "auto.runs.did.assign_owner",
    queued: "auto.runs.queued.assign_owner",
  },
  advance_deal: {
    did: "auto.runs.did.advance_deal",
    queued: "auto.runs.queued.advance_deal",
  },
  send_email: {
    did: "auto.runs.did.send_email",
    queued: "auto.runs.queued.send_email",
  },
  emit_flow_event: {
    did: "auto.runs.did.emit_flow_event",
    queued: "auto.runs.queued.emit_flow_event",
  },
  recompute_score: {
    did: "auto.runs.did.recompute_score",
    queued: "auto.runs.queued.recompute_score",
  },
  enqueue_job: {
    did: "auto.runs.did.enqueue_job",
    queued: "auto.runs.queued.enqueue_job",
  },
  notify: { did: "auto.runs.did.notify", queued: "auto.runs.queued.notify" },
  draft_email: {
    did: "auto.runs.did.draft_email",
    queued: "auto.runs.queued.draft_email",
  },
  add_list_member: {
    did: "auto.runs.did.add_list_member",
    queued: "auto.runs.queued.add_list_member",
  },
};

// A created task named by its subject.
const TITLED_TASK: Words = {
  did: "auto.runs.did.create_task_titled",
  queued: "auto.runs.queued.create_task_titled",
};

// A target the reader may not see, by its kind and never by its id.
export const UNNAMED_TARGET: Readonly<Record<string, MessageKey>> = {
  contact: "auto.runs.unnamed.contact",
  company: "auto.runs.unnamed.company",
  deal: "auto.runs.unnamed.deal",
  lead: "auto.runs.unnamed.lead",
  project: "auto.runs.unnamed.project",
  activity: "auto.runs.unnamed.activity",
};

export function unnamedTarget(type: string | undefined, t: Translator): string {
  const key = type ? UNNAMED_TARGET[type] : undefined;
  return t(key ?? "auto.runs.unnamed.record");
}

/**
 * One sentence per action the run applied or queued, split around its
 * `{target}`. Undefined when the run carries no kinds, or one this client has
 * no words for: the run then reads from its server lines.
 */
export function runSentences(
  run: AutomationRun,
  t: Translator,
): (readonly [string, string])[] | undefined {
  const kinds = run.action_kinds ?? [];
  const mood =
    run.outcome === "fired"
      ? "did"
      : run.outcome === "queued_for_approval"
        ? "queued"
        : undefined;
  if (!mood || kinds.length === 0) return undefined;
  const words = kinds.map((kind) => RUN_ACTION_WORDS[kind]);
  if (words.some((word) => word === undefined)) return undefined;
  const title = run.created?.label;
  return kinds.map((kind, i) => {
    const titled = kind === "create_task" && title !== undefined;
    const key = titled ? TITLED_TASK[mood] : words[i]?.[mood];
    // Split before the title goes in, so a title holding "{target}" stays text.
    const [before = "", after = ""] = (key ? t(key) : "").split("{target}");
    const fill = (part: string) =>
      titled ? part.replace("{title}", () => title ?? "") : part;
    return [fill(before), fill(after)] as const;
  });
}

/** What fired the run, in words, or undefined for a trigger this client predates. */
export function runTrigger(
  run: AutomationRun,
  t: Translator,
): string | undefined {
  const trigger = run.trigger_event_type;
  if (!trigger) return undefined;
  if (trigger === "deal.stage_changed" && run.trigger_stage_name) {
    return t("auto.runs.when.stageNamed", { stage: run.trigger_stage_name });
  }
  return knownTriggerLabel(trigger, t);
}
