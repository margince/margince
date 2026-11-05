// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { LEAD_STATUS_LABEL } from "../../format/leadstatus";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

// The server names a group in the installation's base language. A group that
// is a fixed word is read in the reader's; a name read from a record stays.
const PARTNER_ROLE: Readonly<Record<string, MessageKey>> = {
  sourced: "reporting.group.sourced",
  influenced: "reporting.group.influenced",
};

const PARTNER_ACTIVITY: Readonly<Record<string, MessageKey>> = {
  active: "reporting.group.active",
  inactive: "reporting.group.inactive",
};

/** The reader's word for a breakdown group, keyed by its group key. */
export function breakdownLabel(
  key: string,
  label: string,
  t: Translator,
): string {
  const [kind, value = "", role] = key.split(":");
  if (kind === "status") {
    const status = Object.entries(LEAD_STATUS_LABEL).find(
      ([candidate]) => candidate === value,
    );
    return status ? t(status[1]) : label;
  }
  if (key === "origin:") return t("lead.originNone");
  if (kind === "activity") {
    const word = PARTNER_ACTIVITY[value];
    return word ? t(word) : label;
  }
  if (kind !== "partner") return label;
  const word = role === undefined ? undefined : PARTNER_ROLE[role];
  const name =
    value === "hidden"
      ? t("reporting.group.partnerHidden")
      : partnerName(label, word);
  return word ? `${name} · ${t(word)}` : name;
}

// The server joins a partner's name and its role with " · ". The role is cut
// from the end, so a separator inside the name stays.
function partnerName(label: string, role: MessageKey | undefined): string {
  const separator = label.lastIndexOf(" · ");
  return role && separator >= 0 ? label.slice(0, separator) : label;
}
