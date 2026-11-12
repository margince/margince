// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One deal Margince changed, as a card on Home. It holds the name and value,
// the changes on one line of steps, and the verbs that answer for them all.
// Only the card in hand is open; a closed one says where the deal ended up.

import { EntityRef } from "../../components/entityref";
import { BundleUndo } from "../../components/marginchanges";
import { AppLink } from "../../design-system/applink";
import { Badge, Card, Checkbox } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import {
  formatDayMonth,
  formatMoneyWhole,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { ENTITY } from "../../lib/entity";
import type { MarginceChangeBundle } from "../../lib/marginchanges";
import { routeHash } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import { ChangeRow, ChangeWhat } from "./brief.doneforyou.change";
import { type DealChanges, undoableShown } from "./brief.doneforyou.groups";

export function DealCard({
  group,
  bundle,
  currency,
  open,
  onOpen,
  asOf,
  onSettled,
  pick,
}: Readonly<{
  group: DealChanges;
  bundle: MarginceChangeBundle | undefined;
  /** The currency the summary counts deal values in. */
  currency: string | undefined;
  open: boolean;
  onOpen: (open: boolean) => void;
  asOf: string | undefined;
  onSettled: (id: string) => void;
  /** The tick that puts the deal in the Undo of many, where one is offered. */
  pick?: Readonly<{ on: boolean; onChange: (on: boolean) => void }>;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const newest = group.receipts[0];
  const total = group.receipts.length + group.unloaded;
  const value =
    bundle?.value_base_minor !== undefined && currency
      ? formatMoneyWhole(bundle.value_base_minor, currency, locale)
      : undefined;
  return (
    <li>
      <Card as="article" className="brief-done-deal">
        <Disclosure
          className="brief-done-fold"
          open={open}
          onToggle={onOpen}
          summary={
            <span className="brief-done-head">
              <span className="brief-done-name">
                {pick && (
                  <Checkbox
                    className="brief-done-pick"
                    checked={pick.on}
                    onChange={(event) => pick.onChange(event.target.checked)}
                    label={
                      <span className="sr-only">
                        {t("brief.done.pick", {
                          name: group.name ?? bundle?.deal_name ?? "",
                        })}
                      </span>
                    }
                  />
                )}
                <EntityRef kind="deal" id={group.dealId} name={group.name} />
              </span>
              {!open && newest && (
                <time
                  className="t-caption brief-done-latest"
                  dateTime={newest.occurred_at}
                >
                  {formatDayMonth(newest.occurred_at, locale, zone)}
                </time>
              )}
              <span className="brief-done-chips">
                {value && <Badge>{value}</Badge>}
                {total > 1 && (
                  <Badge tone="ai">
                    {plural("brief.done.changeCount", total, {
                      count: formatNumber(total, locale),
                    })}
                  </Badge>
                )}
              </span>
              {!open && newest && (
                <span className="brief-done-net">
                  <ChangeWhat receipt={newest} />
                </span>
              )}
            </span>
          }
        >
          <ol className="brief-done-changes" aria-label={t("brief.done.steps")}>
            {group.receipts.map((receipt) => (
              <ChangeRow
                key={receipt.id}
                receipt={receipt}
                asOf={asOf}
                onSettled={() => onSettled(receipt.id)}
              />
            ))}
          </ol>
          {group.unloaded > 0 && (
            <p className="t-caption">
              <AppLink href={routeHash(ENTITY.deal.route(group.dealId))}>
                {plural("brief.done.viewChanges", group.unloaded, {
                  count: formatNumber(group.unloaded, locale),
                })}
              </AppLink>
            </p>
          )}
          <DealVerbs group={group} bundle={bundle} onSettled={onSettled} />
        </Disclosure>
      </Card>
    </li>
  );
}

/** The deal's own Undo: every change listed above that the server allows. */
function DealVerbs({
  group,
  bundle,
  onSettled,
}: Readonly<{
  group: DealChanges;
  bundle: MarginceChangeBundle | undefined;
  onSettled: (id: string) => void;
}>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const settleAll = () => {
    for (const receipt of group.receipts) onSettled(receipt.id);
  };
  const audits = undoableShown(group, bundle);
  if (!bundle || audits.length <= 1) return null;
  return (
    <div className="brief-done-verbs">
      <BundleUndo
        bundles={[{ ...bundle, audit_log_ids: audits }]}
        label={plural("brief.done.undoChanges", audits.length, {
          count: formatNumber(audits.length, locale),
        })}
        onDone={settleAll}
      />
    </div>
  );
}
