// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { ScopeChips } from "../design-system/passportselect";
import { useT } from "../i18n";

export type WorkArea = components["schemas"]["WorkArea"];
export type PassportAreas = components["schemas"]["PassportAreas"];
type AreaMode = PassportAreas["mode"];

type Translate = ReturnType<typeof useT>;

export const ALL_AREAS: PassportAreas = { mode: "all", areas: [] };

// One area alone is no choice: the role already keeps the agent there, so the
// section is not drawn and the grant is all areas.
export function offersWorkAreas(reachable: readonly WorkArea[]): boolean {
  return reachable.length >= 2;
}

export function areasComplete(value: PassportAreas): boolean {
  return value.mode === "all" || value.areas.length > 0;
}

// The `areas` field the consent form posts: the literal `all`, or the chosen
// keys separated by single spaces.
export function areasFormValue(value: PassportAreas): string {
  return value.mode === "all" ? "all" : value.areas.join(" ");
}

// The areas a grant reaches today. A custom grant keeps an area the role no
// longer reaches, and the server refuses it there anyway, so it is not counted.
export function effectiveAreas(
  value: PassportAreas,
  reachable: readonly WorkArea[],
): WorkArea[] {
  return value.mode === "all"
    ? [...reachable]
    : reachable.filter((area) => value.areas.includes(area));
}

export function workAreaName(t: Translate, area: WorkArea): string {
  return t(`workArea.name.${area}`);
}

// Areas come back in the order the role lists them, so two equal sets read the
// same and compare equal.
function inReachableOrder(
  areas: readonly WorkArea[],
  reachable: readonly WorkArea[],
): WorkArea[] {
  return reachable.filter((area) => areas.includes(area));
}

/**
 * Where a passport may work: all areas, or the ticked ones. Draws nothing
 * when the role reaches fewer than two areas, and the caller then sends all.
 * A choice of no areas is the caller's to refuse, beside its own submit.
 */
export function WorkAreasChoice({
  reachable,
  value,
  onChange,
  disabled,
}: Readonly<{
  reachable: readonly WorkArea[];
  value: PassportAreas;
  onChange: (next: PassportAreas) => void;
  disabled?: boolean;
}>) {
  const t = useT();
  if (!offersWorkAreas(reachable)) {
    return null;
  }
  // Choosing areas starts from what the grant reaches now, so the switch alone
  // takes nothing away; the reader unticks what to remove.
  const switchMode = (mode: AreaMode) =>
    onChange(
      mode === "all"
        ? ALL_AREAS
        : { mode, areas: effectiveAreas(value, reachable) },
    );
  return (
    <div className="form-stack">
      <ChoiceList<AreaMode>
        legend={t("workArea.legend")}
        layout="cards"
        disabled={disabled}
        value={value.mode}
        onChange={switchMode}
        choices={[
          {
            value: "all",
            label: t("workArea.mode.all"),
            description: t("workArea.mode.allNote"),
          },
          {
            value: "custom",
            label: t("workArea.mode.custom"),
            description: t("workArea.mode.customNote"),
          },
        ]}
      />
      {value.mode === "custom" && (
        <ChoiceList<WorkArea>
          multiple
          legend={t("workArea.pickLegend")}
          hideLegend
          disabled={disabled}
          value={value.areas}
          onChange={(areas) =>
            onChange({
              mode: "custom",
              areas: inReachableOrder(areas, reachable),
            })
          }
          choices={reachable.map((area) => ({
            value: area,
            label: workAreaName(t, area),
            description: t(`workArea.hint.${area}`),
          }))}
        />
      )}
    </div>
  );
}

/** A passport's areas in a list cell: "All areas", or one chip per area. */
export function WorkAreasCell({ areas }: Readonly<{ areas: PassportAreas }>) {
  const t = useT();
  if (areas.mode === "all") {
    return <Badge>{t("workArea.mode.all")}</Badge>;
  }
  return (
    <ScopeChips labels={areas.areas.map((area) => workAreaName(t, area))} />
  );
}
