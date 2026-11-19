// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Editing the choices of a picklist or multiselect field. Each choice shows how
// many records hold it. The save clears a removed choice from those records, so
// the dialog says before Save how many lose it. The toast after says how many
// records changed.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { SurfaceState } from "../../design-system/surfacestate";
import { useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { OptionRows } from "./customfields.optionrows";

type CustomField = components["schemas"]["CustomField"];

export function CustomFieldChoices({
  field,
}: Readonly<{ field: CustomField }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  if (field.type !== "picklist" && field.type !== "multiselect") {
    return null;
  }
  return (
    <>
      <Button aria-haspopup="dialog" onClick={() => setOpen(true)}>
        {t("cf.choices.edit")}
      </Button>
      {open && <ChoicesDialog field={field} onClose={() => setOpen(false)} />}
    </>
  );
}

// The trimmed, non-blank choices in their order, each once.
export function cleanChoices(options: readonly string[]): string[] {
  return [
    ...new Set(options.map((one) => one.trim()).filter((one) => one !== "")),
  ];
}

// How many records lose a choice when `kept` replaces the field's choices.
export function recordsLosingAChoice(
  held: Readonly<Record<string, number>>,
  kept: readonly string[],
): number {
  return Object.entries(held)
    .filter(([choice]) => !kept.includes(choice))
    .reduce((sum, [, records]) => sum + records, 0);
}

function ChoicesDialog({
  field,
  onClose,
}: Readonly<{ field: CustomField; onClose: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [options, setOptions] = useState<string[]>(field.options ?? [""]);
  const use = useQuery({
    queryKey: ["custom-fields", field.object, "choices", field.id],
    queryFn: async () =>
      unwrap(
        await api.GET("/custom-fields/{id}/choices", {
          params: { path: { id: field.id } },
        }),
      ),
  });
  const held = Object.fromEntries(
    (use.data?.choices ?? []).map((one) => [one.choice, one.records]),
  );
  const kept = cleanChoices(options);
  const losing = recordsLosingAChoice(held, kept);
  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/custom-fields/{id}/options", {
          params: { path: { id: field.id } },
          body: { options: kept },
        }),
      ),
    onSuccess: (saved) => {
      queryClient.invalidateQueries({ queryKey: ["custom-fields"] });
      queryClient.invalidateQueries({ queryKey: ["cf-audit"] });
      const changed = saved.records_changed ?? 0;
      toast.show(
        plural("cf.choices.saved", changed, {
          label: field.label,
          count: formatNumber(changed, locale),
        }),
      );
      onClose();
    },
  });
  return (
    <ConfirmModal
      open
      onClose={onClose}
      tier="confirm"
      title={t("cf.choices.title", { label: field.label })}
      confirmLabel={t("trust.save")}
      confirmDisabled={kept.length === 0 || use.isPending}
      pending={save.isPending}
      error={save.isError ? problemMessageOf(save.error, t) : null}
      onConfirm={() => save.mutate()}
    >
      <SurfaceState
        state={
          use.isPending ? "loading" : use.isError ? "unavailable" : "ready"
        }
        emptyLabel=""
        loadingLabel={t("cf.choices.checking")}
        loadingLines={2}
      >
        <OptionRows
          options={options}
          onChange={setOptions}
          aside={(option) =>
            option in held ? (
              <span className="t-caption">
                {plural("cf.choices.held", held[option] ?? 0, {
                  count: formatNumber(held[option] ?? 0, locale),
                })}
              </span>
            ) : null
          }
        />
        {losing > 0 && (
          <Callout
            tone="warning"
            kind="standing"
            title={plural("cf.choices.clears", losing, {
              count: formatNumber(losing, locale),
            })}
          />
        )}
      </SurfaceState>
    </ConfirmModal>
  );
}
