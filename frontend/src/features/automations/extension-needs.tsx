// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Need = components["schemas"]["ComposedExtensionNeed"];
type MailHooks = components["schemas"]["ComposedExtensionMailHooks"];

// A capability this screen has no word for shows as its own name. A grant is
// never hidden from the admin.
function needLabel(capability: string, t: ReturnType<typeof useT>): string {
  switch (capability) {
    case "models":
      return t("extAccess.need.models");
    case "objects":
      return t("extAccess.need.objects");
    default:
      return capability;
  }
}

/** What a unit contributes: objects, routes, jobs, needs and mail hooks. */
export function bringsCount(
  unit: Readonly<{
    rbac_objects: readonly string[];
    routes: readonly unknown[];
    jobs: readonly string[];
    needs?: readonly Need[];
    mail_hooks?: MailHooks;
  }>,
): number {
  return (
    unit.rbac_objects.length +
    unit.routes.length +
    unit.jobs.length +
    (unit.needs?.length ?? 0) +
    (unit.mail_hooks ? 1 : 0)
  );
}

/** What a unit uses from the core, each with the reason its author gave. */
export function UnitNeeds({ needs }: Readonly<{ needs: readonly Need[] }>) {
  const t = useT();
  if (needs.length === 0) {
    return <>{t("extAccess.brings.none")}</>;
  }
  return (
    <ul className="ext-brings-list">
      {needs.map((need) => (
        <li key={need.capability}>
          <strong>{needLabel(need.capability, t)}</strong>
          {` — ${need.reason}`}
        </li>
      ))}
    </ul>
  );
}

/** The needs row of a unit's "brings" list. */
export function needsFact(needs: readonly Need[] | undefined, term: string) {
  return { key: "needs", term, value: <UnitNeeds needs={needs ?? []} /> };
}

/**
 * What of each captured mail a unit's hooks read, as the server reports it. A
 * part this screen has no word for shows as its own name, as a need does.
 */
function UnitMailReads({ hooks }: Readonly<{ hooks: MailHooks }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = (read: string) => {
    switch (read) {
      case "subject":
        return t("extAccess.mail.subject");
      case "addresses":
        return t("extAccess.mail.addresses");
      case "body":
        return plural("extAccess.mail.body", hooks.body_characters, {
          count: formatNumber(hooks.body_characters, locale),
        });
      default:
        return read;
    }
  };
  return (
    <ul className="ext-brings-list">
      {hooks.reads.map((read) => (
        <li key={read}>{label(read)}</li>
      ))}
    </ul>
  );
}

/**
 * The mail row of a unit's "brings" list, present only for a unit with a mail
 * hook. The unit may send what its hooks read to its own provider, so the admin
 * learns of it before granting the unit anything.
 */
export function mailHooksFacts(hooks: MailHooks | undefined, term: string) {
  return hooks
    ? [{ key: "mail", term, value: <UnitMailReads hooks={hooks} /> }]
    : [];
}
