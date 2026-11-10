// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A title that says what is wrong with what it names.
//
// Several lanes send only a name as their title: a deal, a contact, a rule, a
// message subject. "Depot rollout" on a card does not say why it is there, so
// these lanes frame the name in a sentence about their event.

import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { displayedTitle } from "./systemdisplay";
import { partyName } from "./worklistcopyreasons";
import type { WorklistItem } from "./worklisttypes";

type Framing = Readonly<{
  key: MessageKey;
  // The row's `kind` that picks another sentence, where the lane sends one.
  byKind?: Readonly<Record<string, MessageKey>>;
}>;

const FRAMED: Readonly<Partial<Record<WorklistItem["source"], Framing>>> = {
  deal_at_risk: {
    key: "worklist.title.deal_at_risk",
    byKind: { close_overdue: "worklist.title.deal_at_risk.close_overdue" },
  },
  relationship_decay: { key: "worklist.title.relationship_decay" },
  automation_run: {
    key: "worklist.title.automation_run",
    byKind: {
      failed: "worklist.title.automation_run.failed",
      blocked: "worklist.title.automation_run.blocked",
    },
  },
  meeting_outcome: { key: "worklist.title.meeting_outcome" },
  meeting_follow_up: { key: "worklist.title.meeting_follow_up" },
  awaiting_reply: { key: "worklist.title.awaiting_reply" },
  bounce: { key: "worklist.title.bounce" },
  lead_response: { key: "worklist.title.lead_response" },
  customer_waiting: { key: "worklist.title.customer_waiting" },
};

// The lanes whose own title is a message subject, which already names the
// event well enough once the contact is known.
const SUBJECT_TITLED = new Set<WorklistItem["source"]>(["customer_waiting"]);

/** The row's own title, framed where its lane sends only a name. */
export function ownTitle(
  item: WorklistItem,
  t: Translator,
): string | undefined {
  return framedTitle(item, t) ?? item.title;
}

function framedTitle(item: WorklistItem, t: Translator): string | undefined {
  const framing = Object.hasOwn(FRAMED, item.source)
    ? FRAMED[item.source]
    : undefined;
  if (!framing) return undefined;
  const own = displayedTitle(item)?.trim();
  if (own && SUBJECT_TITLED.has(item.source)) return undefined;
  const name = own || item.subject?.label?.trim() || partyName(item);
  if (!name) return undefined;
  const kind = item.kind ?? "";
  const key =
    framing.byKind && Object.hasOwn(framing.byKind, kind)
      ? framing.byKind[kind]
      : framing.key;
  return t(key ?? framing.key, { name });
}
