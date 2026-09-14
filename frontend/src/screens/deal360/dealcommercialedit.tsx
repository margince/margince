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
  // `null` is "no recurring value", which is a different answer from 0 — a
  // deal explicitly worth nothing per year. The server keeps both and refuses
  // only a negative, so the form has to keep both too.
  const [arrMinor, setArrMinor] = useState<number | null>(null);
  // The reading this modal OPENED on, used to pin the save. `deal.version`
  // moves under an open modal when the query refetches, and pinning to the
  // live one makes the conflict check pass while the form still holds
  // pre-refetch values — so a colleague's change to a field this reader never
  // touched is silently overwritten by the stale copy.
  const [openedVersion, setOpenedVersion] = useState<number | undefined>();
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
    setArrMinor(current.expected_arr_minor ?? null);
    setOpenedVersion(current.version);
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
  // An ARR the deal took FROM AN ACCEPTED OFFER is not a figure to retype. The
  // server refuses a manual edit or clear while that provenance stands
  // (`ArrFromOfferError`), and it refuses the WHOLE patch — so an editor here
  // would lose a reader's unrelated motion or priority change along with it.
  const arrFromOffer = Boolean(deal.arr_source_offer_id);
  const canPriceArr = !moneyMasked && currency !== null && !arrFromOffer;

  async function submit() {
    const body: UpdateDealRequest = {
      commercial_motion: commercialMotion(motion),
      priority: dealPriority(priority),
      // An empty pick is "nobody answered", which the wire spells as null.
      acquisition_source: source === "" ? null : source,
    };
    // Only when this reader was actually shown the money AND may change it.
    // Omitting the field leaves it alone; sending a figure they never entered
    // would clear or restate one they may not read or may not touch.
    if (canPriceArr) {
      body.expected_arr_minor = arrMinor;
    }
    // Pinned to the reading the FORM holds, not the live one. See openedVersion.
    await save.mutateAsync({ version: openedVersion, body });
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
              <div className="actionrow">
                <MoneyInput
                  {...control}
                  // A deal with NO recurring value shows nothing, and one worth
                  // zero per year shows 0.00 — the server keeps those apart and
                  // so does this. `blankWhenZero` would collapse them.
                  valueMinor={arrMinor ?? 0}
                  currency={currency}
                  onChangeMinor={setArrMinor}
                  blankWhenZero={arrMinor === null}
                  disabled={save.isPending}
                />
                {/* MoneyInput deliberately ignores an emptied box — a
                    half-deleted buffer is not an intention, and it restores the
                    last committed figure on blur. Its own comment says the
                    remedy is an explicit control that means "remove this
                    value", so this is that control. Without it there is no way
                    to take an ARR back off a deal. */}
                {arrMinor !== null && (
                  <Button
                    variant="ghost"
                    onClick={() => setArrMinor(null)}
                    disabled={save.isPending}
                  >
                    {t("deal.arrClear")}
                  </Button>
                )}
              </div>
            )}
          </Field>
        )}
        {arrFromOffer && !moneyMasked && (
          // The figure came from an accepted offer, and the server refuses a
          // manual edit while that stands. Saying so beats a refused save that
          // would also lose the reader's unrelated changes.
          <p className="t-caption mute">{t("deal.arrFromOffer")}</p>
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
          <Button
            onClick={() => {
              // The mutation already HOLDS the failure — the alert above renders
              // from its `isError`. What is swallowed here is only the promise
              // `mutateAsync` returns, which is otherwise an unhandled rejection
              // for a refusal the reader is already looking at.
              submit().catch(() => {});
            }}
            disabled={save.isPending}
          >
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
