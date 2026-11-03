// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, type LucideIcon, Mail } from "lucide-react";
import { createElement } from "react";
import type { components } from "../api/schema";
import type { Choice } from "../design-system/choicelist";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  type FlowAccount,
  providerFor,
  type Service,
} from "./connect-flow-draft";

// What Margince should read from one account, asked in the connect dialog, in
// "Change what it brings in" and in onboarding. One builder serves all three,
// so they never word or refuse a service differently.

type Provider = components["schemas"]["CaptureConnection"]["provider"];

export const SERVICE_ICON: Record<Service, LucideIcon> = {
  mail: Mail,
  calendar: CalendarDays,
};

/**
 * What each service is called and what it brings in, as catalog keys.
 *
 * Keys rather than words, so a caller translates them with its own translator.
 * Two screens show the same row. A settings page also says when it last
 * synced; onboarding says what the step is for instead. Spelled twice, a
 * renamed key reaches one of them.
 */
export const SERVICE_WORDS: Record<
  Service,
  { name: MessageKey; desc: MessageKey }
> = {
  mail: { name: "connectors.svc.newMail", desc: "connectors.svc.newMailDesc" },
  calendar: {
    name: "connectors.svc.calendar",
    desc: "connectors.svc.calendarDesc",
  },
};

/**
 * Mail and Calendar as choices for `account`. A service the roster in
 * `connected` already holds is refused as already connected, and a calendar
 * over IMAP is refused because that protocol carries mail alone.
 */
export function serviceChoices(
  t: Translator,
  {
    account = "google",
    connected = [],
  }: Readonly<{ account?: FlowAccount; connected?: readonly Provider[] }> = {},
): Choice<Service>[] {
  const imap = account === "imap";
  const has = (service: Service) => {
    const provider = providerFor(account, service);
    return provider !== null && connected.includes(provider);
  };
  const mailHelp = imap ? "connectFlow.mailHelpImap" : "connectFlow.mailHelp";
  let calendarHelp = t("connectFlow.calendarHelp");
  if (imap) {
    calendarHelp = t("connectFlow.calendarImap");
  } else if (has("calendar")) {
    calendarHelp = t("connectFlow.alreadyConnected");
  }
  return [
    {
      value: "mail",
      label: t("connectFlow.mail"),
      description: has("mail")
        ? t("connectFlow.alreadyConnected")
        : t(mailHelp),
      mark: createElement(SERVICE_ICON.mail),
      disabled: has("mail"),
    },
    {
      value: "calendar",
      label: t("connectFlow.calendar"),
      description: calendarHelp,
      mark: createElement(SERVICE_ICON.calendar),
      disabled: imap || has("calendar"),
    },
  ];
}
