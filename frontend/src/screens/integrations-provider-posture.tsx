// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { PostureRefused } from "./integrations-provider.notices";

// The installation's lookup posture, read and written through its own surface.
//
// NOT the connection's configuration. Whether contacts are looked up without
// anybody asking is one answer for the installation, and the three
// per-connection fields that used to carry it are deprecated and ignored by
// admission. A card that still PATCHed them would save successfully, answer
// 200, and change nothing — worse than a missing control, because the screen
// would be telling the reader the opposite of what the system does.
function useLookupPosture() {
  return useQuery({
    queryKey: ["integrations-settings"],
    queryFn: async () => {
      const { data, error } = await api.GET("/integrations/settings");
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function usePatchLookupPosture() {
  const queryClient = useQueryClient();
  return useMutation({
    // The posture travels as the mutation's variable rather than closing over
    // render state: the switch that was pressed is the one that must be saved,
    // even if the card re-rendered while the write was in flight.
    mutationFn: async (automaticLookup: boolean) => {
      const { data, error } = await api.PATCH("/integrations/settings", {
        body: { automatic_lookup: automaticLookup },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: ["integrations-settings"],
      });
      // The backlog rides the connection, and the posture decides whether it
      // is paused — so the card's other half has to re-read too.
      void queryClient.invalidateQueries({
        queryKey: ["provider-connections"],
      });
    },
  });
}

// The lookup switch is the control here that a reader who may not change it
// still needs to READ: this is the only place the installation says whether
// contacts are being looked up at somebody's expense. So it is neither absent
// (that would hide a granted read) nor withheld (there is a fact to show) — it
// is the shape the design system keeps for exactly this: a Switch, because
// flipping it writes, with `reason` carrying the denial to a screen reader
// through aria-describedby rather than leaving it beside the control as
// decoration.
//
// ONE switch, where there were two. Those asked which WRITER a purchase
// followed — a colleague typing a contact, a connector importing one — and the
// answer differed because a connector's thousands of contacts each spent
// credits. A lookup now buys only what the provider gives away, so that
// distinction stopped paying for itself, and what is left is a question about
// the installation rather than about the writer.
//
// Drawn ONCE, above the connections: the answer belongs to the installation,
// and inside the per-connection loop a second registered provider drew a second
// copy of the same switch.
export function LookupPostureRow({ canEdit }: Readonly<{ canEdit: boolean }>) {
  const t = useT();
  const posture = useLookupPosture();
  const patch = usePatchLookupPosture();
  return (
    <>
      <SettingRow
        label={t("provider.automaticLookup")}
        // Two paragraphs, not one string with a blank line in it: HTML collapses
        // the break, and the half that would have been glued on is the one an
        // operator in the wrong jurisdiction has to read.
        description={
          <>
            <span className="provider-hint-para">
              {t("provider.automaticLookupHint")}
            </span>
            <span className="provider-hint-para">
              {t("provider.automaticLookupJurisdiction")}
            </span>
          </>
        }
        control={(control) => (
          <Switch
            // The row's description reaches the switch: what the lookup DOES
            // is the sentence on the left, and a node-form control cannot see
            // the id the row minted for it.
            describedBy={control["aria-describedby"]}
            checked={posture.data?.automatic_lookup ?? false}
            onChange={(next) => patch.mutate(next)}
            // Three causes, and only one of them is worth words. A permission
            // is permanent and has to be explained; a write in flight explains
            // itself by finishing, and a posture still loading resolves on its
            // own.
            //
            // The shared single-control sentence, not the card's own posture
            // line: that one names why the CARD is read-only and would say the
            // same thing twice here, once as prose and once attached to the
            // control.
            //
            // NOT disabled while disconnected, unlike the switches this
            // replaces. The answer belongs to the installation rather than to
            // the connection, and an operator deciding it BEFORE connecting a
            // provider is the order this setting is meant to support.
            reason={canEdit ? undefined : t("captureSettings.adminOnly")}
            disabled={!canEdit || posture.isPending || posture.isError}
            pending={patch.isPending}
            // The row already draws this name on the left, so the switch keeps
            // its own copy hidden: it owns its accessible name by design (see
            // switch.tsx) and pointing it at the row's span as well would name
            // it twice.
            label={t("provider.automaticLookup")}
            labelHidden
          />
        )}
      />
      <PostureRefused writeError={patch.error} readError={posture.error} />
    </>
  );
}
