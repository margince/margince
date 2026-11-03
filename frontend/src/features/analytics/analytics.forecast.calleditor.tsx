import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { MoneyInput } from "../../design-system/moneyinput";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { type AnalyticsSelection, writableScope } from "./analytics.context";
import type { ForecastPeriod } from "./forecast.queries";

type Readings = components["schemas"]["ForecastReadings"];

// Recording what somebody believes will close.
//
// It writes no deal row, and the copy says so. A manager who disagrees with the
// derived figure records their own number, rather than edit the pipeline until
// the derivation agrees.
export function ForecastCallEditor({
  readings,
  selection,
  period,
  onClose,
}: Readonly<{
  readings: Readings;
  selection: AnalyticsSelection;
  period: ForecastPeriod;
  // Leaving the editor, whether by cancelling or by a recorded call: the view
  // that opened it owns whether it is open.
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [amountMinor, setAmountMinor] = useState<number>(
    readings.current_call?.amount_minor ?? 0,
  );
  const [note, setNote] = useState("");

  const save = useMutation({
    // Amount and note travel as VARIABLES. A mutation closing over its inputs
    // sends what the last render saw, which is wrong when a save races a refetch.
    mutationFn: async (call: { amountMinor: number; note: string }) => {
      const named = writableScope(selection.scope);
      if (!named) {
        // The editor is not rendered without a nameable population, so this is
        // unreachable rather than a state to design copy for.
        throw new Error("a forecast names one population");
      }
      return unwrap(
        await api.POST("/forecast/calls", {
          body: {
            // The call is filed for the population and the window the reader is
            // looking at, so the call and the figure it came from agree. A call made
            // while reading one team's week must not land on the workspace or a quarter.
            period,
            ...named,
            amount_minor: call.amountMinor,
            currency: readings.base_currency,
            // An empty note is no note. Sent as "", it would claim the author
            // wrote something blank.
            note: call.note === "" ? undefined : call.note,
          },
        }),
      );
    },
    onSuccess: async () => {
      // The readings carry the standing call, so they are stale the moment one
      // is recorded. The whole "forecast" prefix goes, not this population
      // alone: a workspace call changes what a team reading shows beneath it.
      await client.invalidateQueries({ queryKey: ["forecast"] });
      onClose();
    },
  });

  return (
    <Panel
      title={t("forecast.updateCall")}
      // Cancel and save both leave this editor, so they stand under what they
      // act on rather than in the band that names it.
      actions={
        <>
          <Button onClick={onClose}>{t("forecast.cancel")}</Button>
          <Button
            variant="primary"
            disabled={save.isPending}
            onClick={() => save.mutate({ amountMinor, note })}
          >
            {t("forecast.saveCall")}
          </Button>
        </>
      }
    >
      <PanelBody>
        {/* Two sentences: what a call is, and what recording one does not do.
            The head band holds one line, and the half it would cut is the
            half that says no deal moves. */}
        <PanelIntro>{t("forecast.callExplains")}</PanelIntro>
        <Field label={t("forecast.expectedTotal")}>
          {(control) => (
            <MoneyInput
              {...control}
              valueMinor={amountMinor}
              currency={readings.base_currency}
              onChangeMinor={(next) => setAmountMinor(next ?? 0)}
            />
          )}
        </Field>
        <Field label={t("forecast.supportingNote")}>
          {(control) => (
            <TextInput
              {...control}
              type="text"
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
      </PanelBody>
    </Panel>
  );
}
