// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { useT } from "../../i18n";

type Need = components["schemas"]["ComposedExtensionNeed"];

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

/** How much a unit contributes at all: objects, routes, jobs and needs. */
export function bringsCount(
  unit: Readonly<{
    rbac_objects: readonly string[];
    routes: readonly unknown[];
    jobs: readonly string[];
    needs?: readonly Need[];
  }>,
): number {
  return (
    unit.rbac_objects.length +
    unit.routes.length +
    unit.jobs.length +
    (unit.needs?.length ?? 0)
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
