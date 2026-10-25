// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A within_radius clause's operand: a centre point and a distance in
// kilometres. The server compiles it through storekit's radiusOperand, which
// refuses a missing part, a distance of zero or less, and a point off the globe.

import { useState } from "react";
import { useT } from "../../i18n";
import {
  isRadiusOperand,
  type LeafValue,
  type RadiusOperand,
} from "../../lib/segmentpredicate";

type Draft = Readonly<{ label: string; lat: string; lon: string; km: string }>;

function draftOf(value: LeafValue): Draft {
  if (!isRadiusOperand(value)) {
    return { label: "", lat: "", lon: "", km: "" };
  }
  return {
    label: value.label ?? "",
    lat: String(value.lat),
    lon: String(value.lon),
    km: String(value.radius_km),
  };
}

function numberIn(text: string, low: number, high: number): number | undefined {
  if (text.trim() === "") {
    return undefined;
  }
  const parsed = Number(text);
  return Number.isFinite(parsed) && parsed >= low && parsed <= high
    ? parsed
    : undefined;
}

/**
 * The operand the draft states, or undefined while any part is missing or
 * outside what the server accepts. Mirrors that check so the tree never holds
 * an operand the preview would send only to be refused.
 */
export function radiusOf(draft: Draft): RadiusOperand | undefined {
  const lat = numberIn(draft.lat, -90, 90);
  const lon = numberIn(draft.lon, -180, 180);
  const km = numberIn(draft.km, Number.MIN_VALUE, Number.MAX_VALUE);
  if (lat === undefined || lon === undefined || km === undefined) {
    return undefined;
  }
  const label = draft.label.trim();
  return label === ""
    ? { lat, lon, radius_km: km }
    : { lat, lon, radius_km: km, label };
}

/**
 * The text boxes are held here as typed because a half-typed "-" or "48." is
 * not yet a number. The tree gets the operand once it is whole, and the empty
 * value until then, which keeps the preview back.
 */
export function RadiusValue({
  value,
  onChange,
  label,
}: Readonly<{
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label: string;
}>) {
  const t = useT();
  const [draft, setDraft] = useState<Draft>(() => draftOf(value));
  const edit = (part: Partial<Draft>) => {
    const next = { ...draft, ...part };
    setDraft(next);
    onChange(radiusOf(next) ?? "");
  };
  return (
    <fieldset className="filter-radius-value" aria-label={label}>
      <input
        className="input"
        value={draft.km}
        onChange={(event) => edit({ km: event.target.value })}
        aria-label={t("filters.radius.km")}
        placeholder={t("filters.radius.km")}
        inputMode="decimal"
      />
      <input
        className="input"
        value={draft.label}
        onChange={(event) => edit({ label: event.target.value })}
        aria-label={t("filters.radius.place")}
        placeholder={t("filters.radius.place")}
      />
      <input
        className="input"
        value={draft.lat}
        onChange={(event) => edit({ lat: event.target.value })}
        aria-label={t("filters.radius.lat")}
        placeholder={t("filters.radius.lat")}
        inputMode="decimal"
      />
      <input
        className="input"
        value={draft.lon}
        onChange={(event) => edit({ lon: event.target.value })}
        aria-label={t("filters.radius.lon")}
        placeholder={t("filters.radius.lon")}
        inputMode="decimal"
      />
    </fieldset>
  );
}
