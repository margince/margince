// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { UseQueryResult } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { Button, EmptyState, Modal } from "../design-system/atoms";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Panel } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import {
  AwaitingContacts,
  SenderDecisions,
  useAwaitingDecision,
  useSenders,
} from "./capture-senders";
import { HeldThreads, useHeldThreads } from "./held-threads";
import { NetworkReach, reachCaveat, useLinkedInReach } from "./linkedin-reach";

// What the reader's mailboxes and LinkedIn import produced, one row per list.
// Each list is long and read rarely, so the row says how much there is and
// the list opens in a drawer.

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
  const waitingCount = waiting.data?.data.length ?? 0;
  const reachCount = reach.data
    ? (reach.data.accounts ?? []).length
    : undefined;
  // With nothing reached, the caveat is the one fact the reader can act on.
  const reachNote =
    reach.data && reachCount === 0
      ? reachCaveat(t, locale, reach.data, 0)
      : undefined;

  return (
    <Panel title={t("connectionOutcomes.title")}>
      <SettingList bleed="settings" testId="connection-outcomes">
        <OutcomeRow
          label={t("senders.title")}
          description={t("senders.sub")}
          query={senders}
          count={senders.data?.data.length}
          unit="connectionOutcomes.senders"
          onView={() => setOpen("senders")}
        />
        {/* A queue, not a list: shown only while somebody waits on the reader. */}
        {waitingCount > 0 && (
          <OutcomeRow
            label={t("senders.waiting.title")}
            description={t("senders.waiting.sub")}
            query={waiting}
            count={waitingCount}
            unit="connectionOutcomes.contacts"
            onView={() => setOpen("waiting")}
          />
        )}
        <OutcomeRow
          label={t("heldThreads.title")}
          description={t("heldThreads.sub")}
          query={held}
          count={held.data?.data.length}
          unit="connectionOutcomes.threads"
          onView={() => setOpen("held")}
        />
        <OutcomeRow
          label={t("linkedinReach.title")}
          description={reachNote ?? t("linkedinReach.sub")}
          query={reach}
          count={reachCount}
          unit="connectionOutcomes.companies"
          onView={() => setOpen("reach")}
        />
      </SettingList>
      {open === "senders" && senders.data && (
        <OutcomeDrawer title={t("senders.title")} onClose={close}>
          {senders.data.data.length === 0 ? (
            <EmptyState>{t("senders.emptyBody")}</EmptyState>
          ) : (
            <SenderDecisions rows={senders.data.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "waiting" && waiting.data && (
        <OutcomeDrawer title={t("senders.waiting.title")} onClose={close}>
          {waiting.data.data.length === 0 ? (
            <EmptyState>{t("connectionOutcomes.waitingEmpty")}</EmptyState>
          ) : (
            <AwaitingContacts rows={waiting.data.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "held" && held.data && (
        <OutcomeDrawer title={t("heldThreads.title")} onClose={close}>
          {held.data.data.length === 0 ? (
            <EmptyState>{t("heldThreads.empty")}</EmptyState>
          ) : (
            <HeldThreads list={held.data} />
          )}
        </OutcomeDrawer>
      )}
      {open === "reach" && reach.data && (
        <OutcomeDrawer title={t("linkedinReach.title")} onClose={close}>
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
  onView,
}: Readonly<{
  label: string;
  description: string;
  query: Pick<UseQueryResult, "isError">;
  count: number | undefined;
  unit: PluralBase;
  onView: () => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <SettingRow
      label={label}
      description={description}
      value={
        <OutcomeCount
          failed={query.isError}
          count={count}
          reading={(n) => plural(unit, n, { count: formatNumber(n, locale) })}
        />
      }
      control={
        count !== undefined &&
        count > 0 && (
          <Button
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

function OutcomeCount({
  failed,
  count,
  reading,
}: Readonly<{
  failed: boolean;
  count: number | undefined;
  reading: (count: number) => string;
}>) {
  const t = useT();
  if (failed) return t("connectionOutcomes.unread");
  if (count === undefined) return null;
  return count === 0 ? t("connectionOutcomes.none") : reading(count);
}

function OutcomeDrawer({
  title,
  onClose,
  children,
}: Readonly<{ title: string; onClose: () => void; children: ReactNode }>) {
  const headingId = useId();
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="drawer">
      <DrawerHead>
        <Heading id={headingId} as="h2" size="medium" className="modal-title">
          {title}
        </Heading>
      </DrawerHead>
      <DrawerBody>{children}</DrawerBody>
    </Modal>
  );
}
