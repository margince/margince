// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useState } from "react";
import { Button } from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
} from "../../../design-system/overviewcard";
import { RecordPicker } from "../../../design-system/recordpicker";
import { Select } from "../../../design-system/select";
import { useT } from "../../../i18n";
import { useCanWriteRecord } from "../../../lib/capability";
import { recordRoute } from "../../../lib/entity";
import {
  type LinkedRecordType,
  type LinkType,
  linkTypeLabel,
  type RecordLink,
  useLinkTypes,
  useRecordLinks,
  useRecordLinkWrites,
} from "../../../lib/recordlinks.queries";
import { useRecordTargets } from "../../../lib/recordtargets";
import { routeHash } from "../../../lib/router";

const LINK_ENDS: readonly string[] = ["deal", "company", "contact"];

function isLinkEnd(kind: string | null): kind is LinkedRecordType {
  return kind !== null && LINK_ENDS.includes(kind);
}

/**
 * The deal's typed links to other deals, companies and contacts, read from
 * either end. A reader who may change the deal can add and remove links. The
 * server decides again, so a refusal shows under the card.
 */
export function DealLinksCard({
  deal,
}: Readonly<{ deal: { id: string; writable?: boolean } }>) {
  const t = useT();
  const links = useRecordLinks("deal", deal.id);
  const types = useLinkTypes().data;
  const { create, remove } = useRecordLinkWrites();
  const canWrite = useCanWriteRecord("deal", deal);
  const [adding, setAdding] = useState(false);
  const rows = links.data ?? [];
  return (
    <OverviewCard
      title={t("deal.links.card")}
      action={
        canWrite && !adding ? (
          <Button variant="ghost" onClick={() => setAdding(true)}>
            {t("deal.links.add")}
          </Button>
        ) : undefined
      }
      empty={links.isError ? t("deal.links.unavailable") : t("deal.links.none")}
    >
      {rows.length > 0 || adding ? (
        <>
          <OverviewList>
            {rows.map((link) => (
              <LinkRow
                key={link.id}
                link={link}
                dealId={deal.id}
                types={types}
                canWrite={canWrite}
                removing={remove.isPending && remove.variables === link.id}
                onRemove={() => remove.mutate(link.id)}
              />
            ))}
          </OverviewList>
          {adding && (
            <AddLink
              dealId={deal.id}
              types={types ?? []}
              pending={create.isPending}
              onPick={(body) =>
                create.mutate(body, { onSuccess: () => setAdding(false) })
              }
              onCancel={() => setAdding(false)}
            />
          )}
          <ErrorLine error={create.error ?? remove.error} />
        </>
      ) : undefined}
    </OverviewCard>
  );
}

function LinkRow({
  link,
  dealId,
  types,
  canWrite,
  removing,
  onRemove,
}: Readonly<{
  link: RecordLink;
  dealId: string;
  types: readonly LinkType[] | undefined;
  canWrite: boolean;
  removing: boolean;
  onRemove: () => void;
}>) {
  const t = useT();
  const outgoing = link.from_type === "deal" && link.from_id === dealId;
  const other = outgoing
    ? { type: link.to_type, id: link.to_id, name: link.to_name }
    : { type: link.from_type, id: link.from_id, name: link.from_name };
  const label = linkTypeLabel(types, link.link_type);
  const name = other.name ?? other.id;
  const route = recordRoute(other.type, other.id);
  const target = route ? <a href={routeHash(route)}>{name}</a> : name;
  return (
    <OverviewRow
      title={target}
      sub={t(outgoing ? "deal.links.outgoing" : "deal.links.incoming", {
        type: label,
      })}
      right={
        canWrite ? (
          <Button variant="ghost" pending={removing} onClick={onRemove}>
            {t("deal.links.remove")}
          </Button>
        ) : undefined
      }
    />
  );
}

function AddLink({
  dealId,
  types,
  pending,
  onPick,
  onCancel,
}: Readonly<{
  dealId: string;
  types: readonly LinkType[];
  pending: boolean;
  onPick: (body: {
    from_type: "deal";
    from_id: string;
    to_type: LinkedRecordType;
    to_id: string;
    link_type: string;
  }) => void;
  onCancel: () => void;
}>) {
  const t = useT();
  const startable = types.filter((type) => type.from.includes("deal"));
  const [typeName, setTypeName] = useState("");
  const chosen = startable.find((type) => type.name === typeName);
  const { search, kindOf } = useRecordTargets();
  // Only the records the chosen type may end on, so a pick is never one the
  // server would refuse for its shape.
  const searchTargets = useCallback(
    async (q: string) =>
      chosen
        ? (await search(q)).filter((candidate) => {
            const kind = kindOf(candidate.id);
            return isLinkEnd(kind) && chosen.to.includes(kind);
          })
        : [],
    [chosen, search, kindOf],
  );
  return (
    <div className="deal-links-add">
      <Select
        aria-label={t("deal.links.type")}
        placeholder={t("deal.links.typePlaceholder")}
        value={typeName}
        onChange={setTypeName}
        options={startable.map((type) => ({
          value: type.name,
          label: type.label,
        }))}
      />
      {chosen && (
        <RecordPicker
          label={t("deal.links.search")}
          searchTargets={searchTargets}
          disabled={pending}
          onPick={(candidate) => {
            const kind = kindOf(candidate.id);
            if (isLinkEnd(kind)) {
              onPick({
                from_type: "deal",
                from_id: dealId,
                to_type: kind,
                to_id: candidate.id,
                link_type: chosen.name,
              });
            }
          }}
        />
      )}
      <Button variant="ghost" onClick={onCancel}>
        {t("deal.links.cancel")}
      </Button>
    </div>
  );
}
