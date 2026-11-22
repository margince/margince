// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { parsePeriod } from "../../format/jurisdiction";
import { useLocale, useT } from "../../i18n";
import { floorText } from "../../lib/jurisdictiontext";
import { blankRequired, problemMessageOf, unwrap } from "../../lib/problem";
import {
  JURISDICTION_QUERY_KEY,
  jurisdictionOptions,
} from "./jurisdictionentities";

// Changing the installation's jurisdiction: the choice, and what the change
// puts in force before it is sent.

type Jurisdiction = components["schemas"]["InstallationJurisdiction"];
type RetentionClass = components["schemas"]["JurisdictionRetentionClass"];

// The Select's value for "no pack". The empty string is the Select's "nothing
// chosen", so the explicit choice needs a value of its own; it is sent as "".
const NO_PACK = "none";

export function ChangeJurisdiction({
  jurisdiction,
  reasonId,
}: Readonly<{ jurisdiction: Jurisdiction; reasonId: string | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const [choice, setChoice] = useState("");
  const [open, setOpen] = useState(false);
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const choiceLabel = t("jurisdiction.card.title");
  const current = jurisdiction.selected ?? "";
  const change = useMutation({
    mutationFn: async (country: string) =>
      unwrap(await api.PATCH("/installation/settings", { body: { country } })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: JURISDICTION_QUERY_KEY });
      setOpen(false);
      setChoice("");
    },
  });
  const close = () => {
    setOpen(false);
    setChoice("");
    change.reset();
    faults.show(new Map());
  };
  const target = choice === NO_PACK ? "" : choice;
  const options = [
    ...jurisdictionOptions(jurisdiction.available, locale),
    { value: NO_PACK, label: t("jurisdiction.option.none") },
  ].filter(
    (option) => (option.value === NO_PACK ? "" : option.value) !== current,
  );
  return (
    <>
      <Button
        aria-haspopup="dialog"
        aria-label={t("jurisdiction.change.label")}
        // The row's description is the lock's reason while it holds.
        reasonId={reasonId}
        onClick={() => setOpen(true)}
      >
        {t("jurisdiction.change.action")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={close}
        title={t("jurisdiction.change.label")}
        confirmLabel={t("jurisdiction.confirm.confirm")}
        bodyRef={faultsRoot}
        onConfirm={() => {
          const blank = blankRequired(locale, {
            choice: [choiceLabel, choice],
          });
          if (!faults.show(blank)) change.mutate(target);
        }}
        pending={change.isPending}
        error={change.isError ? problemMessageOf(change.error, t) : null}
      >
        <Field
          label={choiceLabel}
          hint={t("jurisdiction.change.description")}
          required
          error={faults.of("choice")}
        >
          {(control) => (
            <Select
              {...control}
              options={options}
              value={choice}
              placeholder={t("jurisdiction.change.placeholder")}
              onChange={(next) => {
                setChoice(next);
                faults.edited("choice");
                change.reset();
              }}
            />
          )}
        </Field>
        {choice !== "" && (
          <ChangeConsequences jurisdiction={jurisdiction} target={target} />
        )}
      </ConfirmModal>
    </>
  );
}

/** What the confirm dialog lists: the floor before and after, and the rest. */
export function ChangeConsequences({
  jurisdiction,
  target,
}: Readonly<{ jurisdiction: Jurisdiction; target: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const before = jurisdiction.floor.known
    ? floorText(
        {
          name: jurisdiction.floor.class ?? "",
          keep: jurisdiction.floor.keep ?? "P0D",
          from_year_end: jurisdiction.floor.from_year_end,
        },
        locale,
        t,
      )
    : t("jurisdiction.floor.none");
  const afterClass = targetFloor(jurisdiction, target);
  const after = afterClass
    ? floorText(afterClass, locale, t)
    : t("jurisdiction.floor.none");
  const weakens = jurisdiction.floor.known && !afterClass;
  return (
    <>
      <p>{t("jurisdiction.confirm.floor", { from: before, to: after })}</p>
      {weakens && (
        <p>{t("jurisdiction.confirm.floorCarried", { from: before })}</p>
      )}
      <p>{t("jurisdiction.confirm.deadlines")}</p>
      <p>{t("jurisdiction.confirm.messaging")}</p>
    </>
  );
}

// The floor a target puts in force. "No pack" puts the strictest compiled-in
// one in force, which is the longest floor among the available packs.
function targetFloor(
  jurisdiction: Jurisdiction,
  target: string,
): RetentionClass | undefined {
  if (target !== "") {
    return jurisdiction.available.find((unit) => unit.code === target)
      ?.correspondence_floor;
  }
  return jurisdiction.available
    .map((unit) => unit.correspondence_floor)
    .filter((floor): floor is RetentionClass => floor !== undefined)
    .sort((a, b) => periodMonths(b.keep) - periodMonths(a.keep))[0];
}

function periodMonths(iso: string): number {
  const period = parsePeriod(iso);
  return period ? period.years * 12 + period.months + period.days / 31 : 0;
}
