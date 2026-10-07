// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge, Field } from "../design-system/atoms";
import { Select, type SelectOption } from "../design-system/select";
import { stable } from "../format/collate";
import { type Translator, useT } from "../i18n";
import { type ProviderLocations, useProviderLocations } from "./ai-models";

// Where Google processes a Gemini-on-Vertex call. The location IS the
// residency question, so the picker says which jurisdiction each one is under
// and marks the ones that are EU resident.

export const VERTEX_PROVIDER = "gemini_vertex";

/** Google's EU multi-region: the answer that is resident wherever it lands. */
export const DEFAULT_VERTEX_LOCATION = "eu";

type ProviderLocation = components["schemas"]["ProviderLocation"];

/**
 * Where a lane newly pointed at Vertex starts: the location another SAVED
 * Vertex lane already uses, so one installation does not scatter its lanes
 * across jurisdictions by accident.
 */
export function savedVertexLocation(
  saved: readonly { provider: string; location?: string }[],
): string {
  const vertex = saved.find(
    (b) => b.provider === VERTEX_PROVIDER && b.location,
  );
  return vertex?.location ?? DEFAULT_VERTEX_LOCATION;
}
type Jurisdiction = ProviderLocation["jurisdiction"];

const JURISDICTION_ORDER: readonly Jurisdiction[] = [
  "eu",
  "us",
  "other",
  "global",
];

/**
 * The options, grouped EU / US / Other / Global. A location the list does not
 * name — the stored one while the list is loading or cannot be read — is still
 * offered, so the field never shows a value it has no option for.
 */
export function locationOptions(
  list: ProviderLocations | undefined,
  current: string,
  t: Translator,
): SelectOption[] {
  const known = [...(list?.locations ?? [])].sort(
    (a, b) =>
      JURISDICTION_ORDER.indexOf(a.jurisdiction) -
        JURISDICTION_ORDER.indexOf(b.jurisdiction) || stable(a.id, b.id),
  );
  const options: SelectOption[] = known.map((l) => {
    const group = t(`aiRouting.location.group.${l.jurisdiction}`);
    // Google names its multi-regions and leaves every region unnamed.
    const label = l.display_name
      ? t("aiRouting.location.option", {
          group,
          name: l.display_name,
          id: l.id,
        })
      : t("aiRouting.location.optionBare", { group, id: l.id });
    return {
      value: l.id,
      label,
      adornment: l.resident ? (
        <Badge tone="success">{t("aiRouting.location.resident")}</Badge>
      ) : (
        <Badge>{t("aiRouting.location.nonResident")}</Badge>
      ),
    };
  });
  if (current !== "" && !known.some((l) => l.id === current)) {
    options.push({ value: current, label: current });
  }
  return options;
}

function locationHint(
  list: ProviderLocations | undefined,
  pending: boolean,
  noKeyHint: string,
  t: Translator,
): string {
  if (pending) {
    return t("aiRouting.location.loading");
  }
  if (list?.unavailable === "no_key") {
    return noKeyHint;
  }
  if (list?.unavailable) {
    return t("aiRouting.location.unreachable");
  }
  return t("aiRouting.location.help");
}

export function VertexLocationField({
  value,
  disabled,
  noKeyHint,
  onChange,
}: Readonly<{
  value: string;
  disabled: boolean;
  // What to say while no key is held. Settings points at the key card;
  // onboarding is about to save the key itself.
  noKeyHint?: string;
  onChange: (location: string) => void;
}>) {
  const t = useT();
  const locations = useProviderLocations(VERTEX_PROVIDER, true);
  return (
    <Field
      label={t("aiRouting.location.label")}
      hint={locationHint(
        locations.data,
        locations.isPending,
        noKeyHint ?? t("aiRouting.location.noKey"),
        t,
      )}
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          disabled={disabled}
          options={locationOptions(locations.data, value, t)}
          onChange={onChange}
        />
      )}
    </Field>
  );
}
