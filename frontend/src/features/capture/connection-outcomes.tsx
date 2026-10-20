// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type UseQueryResult,
  useIsFetching,
  useIsMutating,
} from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { Button, EmptyState, Modal, Skeleton } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Panel } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber } from "../../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import {
  NetworkReach,
  reachCaveat,
  useLinkedInReach,
} from "../import/linkedin-reach";
import {
  AwaitingContacts,
  SenderDecisions,
  useAwaitingDecision,
  useSenders,
} from "./capture-senders";
import { HeldThreads, useHeldThreads } from "./held-threads";

// One row per list the mailboxes and LinkedIn import produced. Each list is
// long and read rarely, so the row says how much there is and opens a drawer.

type Outcome = "senders" | "waiting" | "held" | "reach";

export function ConnectionOutcomesCard() {
  const t = useT();
  const { locale } = useLocale();
  const senders = useSenders();
  const waiting = useAwaitingDecision();
  const held = useHeldThreads();
  const reach = useLinkedInReach();
  const [open, setOpen] = useState<Outcome | null>(null);
  const close = () => setOpen(null);
  const titleId = useId();
  const viewBase = useId();
  const viewId = (outcome: Outcome) => `${viewBase}-${outcome}`;
  const waitingCount = waiting.data?.data.length ?? 0;
  const reachCount = reach.data?.accounts_total;
  const counts: Record<Outcome, number | undefined> = {
    senders: senders.data?.data.length,
    waiting: waitingCount,
    held: held.data?.data.length,
    reach: reachCount,
  };
  // A list emptied in the drawer, or about to re-read, may take its row's View
  // button away. Focus then goes to the card instead.
  const writing = useIsMutating();
  const reading = useIsFetching();
  const settling = writing + reading > 0;
  const backTo = (outcome: Outcome) => () =>
    (!settling && (counts[outcome] ?? 0) > 0
      ? document.getElementById(viewId(outcome))
      : null) ?? document.getElementById(titleId);
  // With nothing reached, the caveat is the one fact the reader can act on.
  const reachNote =
    reach.data && reachCount === 0
      ? reachCaveat(t, locale, reach.data, 0)
      : undefined;

  return (
    <Panel
      title={
        <span id={titleId} tabIndex={-1}>
          {t("connectionOutcomes.title")}
        </span>
      }
    >
      <SettingList bleed="settings" testId="connection-outcomes">
        <OutcomeRow
          label={t("senders.title")}
          description={t("senders.sub")}
          query={senders}
          count={counts.senders}
          unit="connectionOutcomes.senders"
          viewId={viewId("senders")}
          onView={() => setOpen("senders")}
        />
        {/* A queue, not a list: shown while somebody may wait on the reader. */}
        {(waitingCount > 0 || waiting.isError) && (
          <OutcomeRow
            label={t("senders.waiting.title")}
            description={t("senders.waiting.sub")}
            query={waiting}
            count={waitingCount}
            unit="connectionOutcomes.contacts"
            viewId={viewId("waiting")}
            onView={() => setOpen("waiting")}
          />
        )}
        <OutcomeRow
          label={t("heldThreads.title")}
          description={t("heldThreads.sub")}
          query={held}
          count={counts.held}
          unit="connectionOutcomes.threads"
          viewId={viewId("held")}
          onView={() => setOpen("held")}
        />
        <OutcomeRow
          label={t("linkedinReach.title")}
          description={reachNote ?? t("linkedinReach.sub")}
          query={reach}
          count={reachCount}
          unit="connectionOutcomes.companies"
          viewId={viewId("reach")}
          onView={() => setOpen("reach")}
        />
      </SettingList>
      {open === "senders" && senders.data && (
        <OutcomeDrawer
          title={t("senders.title")}
          onClose={close}
          returnFocusTo={backTo("senders")}
        >
          {senders.data.data.length === 0 ? (
            <EmptyState>{t("senders.emptyBody")}</EmptyState>
          ) : (
            <SenderDecisions rows={senders.data.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "waiting" && waiting.data && (
        <OutcomeDrawer
          title={t("senders.waiting.title")}
          onClose={close}
          returnFocusTo={backTo("waiting")}
        >
          {waiting.data.data.length === 0 ? (
            <EmptyState>{t("connectionOutcomes.waitingEmpty")}</EmptyState>
          ) : (
            <AwaitingContacts rows={waiting.data.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "held" && held.data && (
        <OutcomeDrawer
          title={t("heldThreads.title")}
          onClose={close}
          returnFocusTo={backTo("held")}
        >
          {held.data.data.length === 0 ? (
            <EmptyState>{t("heldThreads.empty")}</EmptyState>
          ) : (
            <HeldThreads list={held.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "reach" && reach.data && (
        <OutcomeDrawer
          title={t("linkedinReach.title")}
          onClose={close}
          returnFocusTo={backTo("reach")}
        >
          <NetworkReach reach={reach.data} />
        </OutcomeDrawer>
      )}
    </Panel>
  );
}

function OutcomeRow({
  label,
  description,
  query,
  count,
  unit,
  viewId,
  onView,
}: Readonly<{
  label: string;
  description: string;
  query: Pick<UseQueryResult, "isPending" | "isError" | "error" | "refetch">;
  count: number | undefined;
  unit: PluralBase;
  viewId: string;
  onView: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (query.isError) {
    return (
      <SettingRow
        label={label}
        description={problemMessageOf(query.error, t)}
        value={t("connectionOutcomes.unread")}
        control={
          <Button
            aria-label={t("connectionOutcomes.retryNamed", { name: label })}
            onClick={() => query.refetch()}
          >
            {t("common.retry")}
          </Button>
        }
      />
    );
  }
  let value: ReactNode = <Skeleton width="5rem" />;
  if (!query.isPending && count !== undefined) {
    value =
      count === 0
        ? t("connectionOutcomes.none")
        : plural(unit, count, { count: formatNumber(count, locale) });
  }
  return (
    <SettingRow
      label={label}
      description={description}
      value={value}
      control={
        count !== undefined &&
        count > 0 && (
          <Button
            id={viewId}
            aria-haspopup="dialog"
            aria-label={t("connectionOutcomes.viewNamed", { name: label })}
            onClick={onView}
          >
            {t("connectionOutcomes.view")}
          </Button>
        )
      }
    />
  );
}

function OutcomeDrawer({
  title,
  onClose,
  returnFocusTo,
  children,
}: Readonly<{
  title: string;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
  children: ReactNode;
}>) {
  const headingId = useId();
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={headingId}
      intent="drawer"
      returnFocusTo={returnFocusTo}
    >
      <DrawerHead>
        <Heading id={headingId} as="h2" size="medium" className="modal-title">
          {title}
        </Heading>
      </DrawerHead>
      <DrawerBody>{children}</DrawerBody>
    </Modal>
  );
}
