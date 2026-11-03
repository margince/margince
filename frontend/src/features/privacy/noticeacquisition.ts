// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { provenanceLabel } from "../../design-system/provenance";
import { provenanceOf } from "../../design-system/provenanceof";
import { acquisitionKindLabel } from "../../format/acquisitionkind";
import { formatDate } from "../../format/format";
import { humanizeToken } from "../../format/humanizetoken";
import type { Locale, Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

export { acquisitionKindLabel } from "../../format/acquisitionkind";

export type NoticeAcquisition = NonNullable<
  components["schemas"]["NoticeAcquisition"]
>;

const RULE: Partial<Record<string, { label: MessageKey; hint: MessageKey }>> = {
  none: { label: "notice.rule.none", hint: "notice.ruleHint.none" },
  unassessed: {
    label: "notice.source.title",
    hint: "notice.source.explanation",
  },
  art13: { label: "notice.rule.art13", hint: "notice.ruleHint.art13" },
  art14: { label: "notice.rule.art14", hint: "notice.ruleHint.art14" },
};

export function noticeRuleLabel(rule: string, t: Translator): string {
  const entry = RULE[rule];
  return entry ? t(entry.label) : humanizeToken(rule);
}

export function noticeRuleHint(
  rule: string,
  t: Translator,
): string | undefined {
  const entry = RULE[rule];
  return entry ? t(entry.hint) : undefined;
}

// Who recorded the evidence. A human whose name no longer resolves is a member
// who left.
export function acquisitionRecorder(
  acquisition: NoticeAcquisition,
  t: Translator,
): string {
  if (acquisition.captured_by_name) {
    return acquisition.captured_by_name;
  }
  const source = provenanceOf(acquisition.captured_by);
  if (source.kind === "human") {
    return t("notice.recorderGone");
  }
  const label = provenanceLabel(source, t, undefined);
  return typeof label === "string" ? label : t("trust.sourceUnknown");
}

// When the contact was obtained, or when it was recorded where nobody said.
export function acquisitionWhen(
  acquisition: NoticeAcquisition,
  t: Translator,
  locale: Locale,
  tz: string,
): string {
  const kind = acquisitionKindLabel(acquisition.kind, t);
  return acquisition.occurred_at
    ? t("notice.acqOn", {
        kind,
        date: formatDate(acquisition.occurred_at, locale, tz),
      })
    : t("notice.acqRecorded", {
        kind,
        date: formatDate(acquisition.captured_at, locale, tz),
      });
}

export function acquisitionCaption(
  acquisition: NoticeAcquisition | null | undefined,
  t: Translator,
  locale: Locale,
  tz: string,
): string {
  if (!acquisition) {
    return t("notice.noAcquisition");
  }
  return [
    acquisitionWhen(acquisition, t, locale, tz),
    acquisitionRecorder(acquisition, t),
  ].join(" · ");
}
