// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { GrantCostNotice } from "../../components/overnight-grant-choice";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { StatusBadge } from "../../design-system/statusbadge";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import {
  type MyAgentGrant,
  morningBriefGrant,
  useAgentGrants,
  useSetAgentGrant,
} from "../../lib/overnight-grant";

// The rep's standing answer: may an agent work as them overnight?
//
// The question is asked in two places. Onboarding asks beside the mailbox
// connect. Settings asks again, because an answer given once has to be findable
// later. Both read and write through lib/overnight-grant.ts so they cannot drift.
//
// Settings writes on flip. Onboarding's switch holds the answer until the step
// continues, and its description says so in words.

// Lapsed and outgrown differ from a decline (the rep already agreed); each
// names its cause, since either degrades the 2am run silently.
function grantStanding(grants: readonly MyAgentGrant[]) {
  const grant = morningBriefGrant(grants);
  const granted = grant?.state === "granted";
  return {
    granted,
    lapsed: granted && !grant?.credential_usable,
    outgrown:
      granted &&
      grant?.credential_usable === true &&
      !grant?.credential_funds_agent,
  };
}

/**
 * The settings card: the same question, answered again later.
 *
 * It writes on flip rather than behind a Save button. It changes only whether
 * an agent works for the one rep flipping it, and is reversible in one click.
 */
export function OvernightGrantCard() {
  const t = useT();
  const query = useAgentGrants();
  const save = useSetAgentGrant();
  // While a flip is in flight the switch shows what the rep just chose, so it
  // does not snap back to the stored answer for the width of a round trip.
  const [optimistic, setOptimistic] = useState<boolean | null>(null);
  // The cost is said when the reader switches it off here. On a later visit the
  // quiet note stands in, because a warning would read their own answer back to
  // them as a fault.
  const [withdrew, setWithdrew] = useState(false);
  const stored =
    query.data === undefined
      ? undefined
      : morningBriefGrant(query.data)?.state === "granted";
  const shownOff = (optimistic ?? stored) === false;

  return (
    <Panel
      title={t("overnightGrant.title")}
      titleAction={
        shownOff ? (
          <StatusBadge kind="neutral" label={t("overnightGrant.off")} />
        ) : null
      }
    >
      <QueryGate query={query} pendingLabel={t("overnightGrant.title")}>
        {(grants) => {
          const { granted, lapsed, outgrown } = grantStanding(grants);
          const shown = optimistic ?? granted;
          return (
            <>
              <SettingList bleed="settings">
                <SettingRow
                  label={t("overnightGrant.label")}
                  description={t("overnightGrant.help")}
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="overnight-grant-toggle"
                      label={t("overnightGrant.label")}
                      labelHidden
                      checked={shown}
                      pending={save.isPending}
                      onChange={(next) => {
                        setOptimistic(next);
                        setWithdrew(!next);
                        save.mutate(next, {
                          onSettled: () => setOptimistic(null),
                        });
                      }}
                    />
                  )}
                />
              </SettingList>
              {!shown && (
                <PanelBody>
                  {withdrew ? (
                    <GrantCostNotice />
                  ) : (
                    <p className="t-caption">{t("overnightGrant.offNote")}</p>
                  )}
                </PanelBody>
              )}
              <RenewalNotices
                showRenewal={lapsed && shown}
                showScopeRenewal={outgrown && shown}
              />
              <PanelNotices
                refused={
                  save.isError
                    ? {
                        title: t("overnightGrant.writeFailedTitle"),
                        error: save.error,
                      }
                    : undefined
                }
              />
            </>
          );
        }}
      </QueryGate>
    </Panel>
  );
}

/** A yes that no longer does the job, kept out of the card so its own body
 * stays the question. */
function RenewalNotices({
  showRenewal,
  showScopeRenewal,
}: Readonly<{ showRenewal: boolean; showScopeRenewal: boolean }>) {
  const t = useT();
  if (!showRenewal && !showScopeRenewal) {
    return null;
  }
  return (
    <PanelBody className="panel-notices">
      {showRenewal && (
        <Callout
          kind="event"
          tone="warning"
          title={t("overnightGrant.renewTitle")}
        >
          {t("overnightGrant.renew")}
        </Callout>
      )}
      {showScopeRenewal && (
        <Callout
          kind="event"
          tone="warning"
          title={t("overnightGrant.renewScopeTitle")}
        >
          {t("overnightGrant.renewScope")}
        </Callout>
      )}
    </PanelBody>
  );
}
