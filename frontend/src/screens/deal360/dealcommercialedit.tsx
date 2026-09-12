import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { Button, Field, Modal } from "../../design-system/atoms";
import { MoneyInput } from "../../design-system/moneyinput";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { AcquisitionSource } from "../acquisitionsources.queries";
import { dealRecordKeys } from "../activitykeys";
import { problemMessageOf, throwProblem } from "../common";
import {
  acquisitionOptions,
  commercialMotion,
  dealPriority,
  MOTION_OPTIONS,
  PRIORITY_OPTIONS,
} from "./dealcommercialfields";

type Deal = components["schemas"]["Deal"];
type UpdateDealRequest = components["schemas"]["UpdateDealRequest"];

/**
 * The deal's commercial context, as a form: why it exists, how much it
 * matters, which channel brought it, and what it is worth per year.
 *
 * A focused PATCH of those four fields rather than a trip through the record's
 * whole edit form, which is where they live today — fifteen fields down, past
 * every other thing a deal has.
 *
 * Expected ARR is only offered when the deal already has a CURRENCY. The
 * server refuses a figure with nothing to price it in
 * (`AmountCurrencyPairError`), and a currency picker here would be this modal
 * quietly re-pricing the deal's amount as well. Setting the currency is the
 * record edit form's job, and the hint says so.
 */
export function DealCommercialEdit({
  open,
  onClose,
  deal,
  sources,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  deal: Deal;
  sources?: AcquisitionSource[];
}>) {
  const t = useT();
  const headingId = useId();
  const [motion, setMotion] = useState("");
  const [priority, setPriority] = useState("");
  const [source, setSource] = useState("");
  const [arrMinor, setArrMinor] = useState(0);
  const save = useSaveCommercial(deal.id);
  // Pulled out because the effect below depends on THIS function rather than
  // on the mutation object, which is a new object every render: depending on
  // the object would reset the form under the reader mid-edit.
  const resetSave = save.reset;
  // The live deal, readable from the effect WITHOUT the effect depending on
  // it. Depending on it would let a background refetch — the deal query
  // refreshes while this modal is open — re-seed the form under a reader
  // mid-edit and replace their unsaved picks with a colleague's.
  const dealRef = useRef(deal);
  dealRef.current = deal;

  // Seeded on the OPENING edge alone. The modal stays mounted for the life of
  // the panel, so an initializer would hand back whatever the deal said the
  // first time the page rendered.
  useEffect(() => {
    if (!open) {
      return;
    }
    const current = dealRef.current;
    setMotion(current.commercial_motion ?? "");
    setPriority(current.priority ?? "");
    setSource(current.acquisition_source ?? "");
    setArrMinor(current.expected_arr_minor ?? 0);
    resetSave();
  }, [open, resetSave]);

  // The money reading is withheld as one unit — amount, ARR and currency
  // together — so a reader who was not shown it must not be offered a control
  // whose save would clear what they cannot see.
  const moneyMasked = Boolean(
    deal.masked_fields?.includes("expected_arr_minor") ||
      deal.masked_fields?.includes("amount_minor") ||
      deal.masked_fields?.includes("currency"),
  );
  const currency = deal.currency ?? null;
  const canPriceArr = !moneyMasked && currency !== null;

  async function submit() {
    const body: UpdateDealRequest = {
      commercial_motion: commercialMotion(motion),
      priority: dealPriority(priority),
      // An empty pick is "nobody answered", which the wire spells as null.
      acquisition_source: source === "" ? null : source,
    };
    // Only when this reader was actually shown the money. Omitting the field
    // leaves it alone; sending a zero the reader never entered would clear a
    // figure they were not permitted to read.
    if (canPriceArr) {
      body.expected_arr_minor = arrMinor === 0 ? null : arrMinor;
    }
    await save.mutateAsync({ version: deal.version, body });
    onClose();
  }

  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId}>
      <h2
        id={headingId}
        className="t-h2"
        style={{ marginBottom: "var(--space-3)" }}
      >
        {t("deal.commercialEditTitle")}
      </h2>
      <div className="form-stack">
        <Field label={t("deal.motion")}>
          {(control) => (
            <Select
              {...control}
              options={MOTION_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={motion}
              onChange={setMotion}
              disabled={save.isPending}
            />
          )}
        </Field>
        <Field label={t("deal.priority")}>
          {(control) => (
            <Select
              {...control}
              options={PRIORITY_OPTIONS.map((o) => ({
                value: o.value,
                label: t(o.label),
              }))}
              value={priority}
              onChange={setPriority}
              disabled={save.isPending}
            />
          )}
        </Field>
        <Field label={t("deal.acquisitionSource")}>
          {(control) => (
            <Select
              {...control}
              options={acquisitionOptions(
                t,
                sources ?? [],
                deal.acquisition_source,
              )}
              value={source}
              onChange={setSource}
              disabled={save.isPending}
            />
          )}
        </Field>
        {canPriceArr && currency && (
          <Field
            label={t("deal.expectedArr")}
            hint={t("deal.arrHint", { currency })}
          >
            {(control) => (
              <MoneyInput
                {...control}
                valueMinor={arrMinor}
                currency={currency}
                onChangeMinor={setArrMinor}
                // An unpriced deal shows an EMPTY box, not "0.00" — a figure
                // nobody entered, which a reader then types after.
                blankWhenZero
                disabled={save.isPending}
              />
            )}
          </Field>
        )}
        {!moneyMasked && currency === null && (
          // The server refuses a figure with no currency to price it in, so
          // the reason is stated here rather than reached through a 422.
          <p className="t-caption mute">{t("deal.arrNeedsCurrency")}</p>
        )}
        {save.isError && (
          <p className="t-caption" role="alert">
            {problemMessageOf(save.error, t)}
          </p>
        )}
        <div className="actions">
          <Button variant="ghost" onClick={onClose} disabled={save.isPending}>
            {t("deals.cancel")}
          </Button>
          <Button onClick={() => void submit()} disabled={save.isPending}>
            {t("deal.commercialSave")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

/**
 * The commercial fields alone, as their own write.
 *
 * Version-pinned like every other deal write: unpinned is last-write-wins, and
 * the loser's edit is gone with success reported to both authors.
 */
function useSaveCommercial(dealId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (args: {
      version: number | undefined;
      body: UpdateDealRequest;
    }) => {
      const { data, error } = await api.PATCH("/deals/{id}", {
        params: {
          path: { id: dealId },
          ...ifMatch(requireVersion(args.version)),
        },
        body: args.body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["deals"] });
      for (const queryKey of dealRecordKeys(dealId)) {
        qc.invalidateQueries({ queryKey });
      }
    },
  });
}
