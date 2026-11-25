// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useT } from "../../i18n";

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
 * The mail row of a unit's "brings" list, present only for a unit with a mail
 * hook. The unit may send what its hooks read to its own provider, so the admin
 * learns of it before granting the unit anything.
 */
export function mailHooksFacts(
  hooks: MailHooks | undefined,
  t: ReturnType<typeof useT>,
) {
  return hooks
    ? [
        {
          key: "mail",
          term: t("extAccess.brings.mail"),
          value: t("extAccess.mail.reads"),
        },
      ]
    : [];
}
