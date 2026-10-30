// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The owner offering their contact or company to a colleague.
//
// It sits in the access panel, under the audience switch, for the owner alone.
// The colleague answers on their Worklist; until then this shows who it is
// offered to and takes the offer back.

import { useState } from "react";
import { Button, Field, Textarea } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { formatDate } from "../../format/format.dates";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useCanMutate } from "../../lib/capability";
import { useMemberName } from "../../lib/membernames";
import { problemMessageOf } from "../../lib/problem";
import { useViewerId } from "../../lib/session";
import {
  type TransferableRecordType,
  useOwnershipOffer,
  useOwnershipOfferActions,
  useOwnershipOfferCandidates,
} from "./ownershiprequest";

type OfferedRecord = Readonly<{
  id?: string;
  owner_id?: string | null;
  writable?: boolean;
  archived_at?: string | null;
}>;

const NOTE_MAX = 500;

/**
 * The offer, for the owner who may still change the record. Nothing renders
 * for anyone else; a manager who is not the owner reassigns it directly.
 */
export function HandoverOffer({
  kind,
  record,
}: Readonly<{ kind: TransferableRecordType; record: OfferedRecord }>) {
  const viewerId = useViewerId();
  const mayOffer =
    useCanMutate() &&
    Boolean(record.id) &&
    viewerId !== undefined &&
    record.owner_id === viewerId &&
    record.writable === true &&
    !record.archived_at;
  const id = record.id ?? "";
  const open = useOwnershipOffer(kind, id, mayOffer);
  if (!mayOffer || open.isPending || open.isError) {
    return null;
  }
  return open.data?.expires_at ? (
    <OfferStanding
      kind={kind}
      id={id}
      toId={open.data.to_id}
      expiresAt={open.data.expires_at}
    />
  ) : (
    <OfferForm kind={kind} id={id} />
  );
}

function OfferStanding({
  kind,
  id,
  toId,
  expiresAt,
}: Readonly<{
  kind: TransferableRecordType;
  id: string;
  toId: string;
  expiresAt: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const toast = useToast();
  const { withdraw } = useOwnershipOfferActions();
  const name = useMemberName(toId);
  return (
    <div>
      <p className="t-caption">
        {t("handover.offered", {
          name: name.data ?? "",
          date: formatDate(expiresAt, locale, viewerZone()),
        })}
      </p>
      <Button
        variant="link"
        pending={withdraw.isPending}
        onClick={() =>
          withdraw.mutate(
            { recordType: kind, id },
            { onSuccess: () => toast.show(t("handover.offerWithdrawn")) },
          )
        }
      >
        {t("handover.withdrawOffer")}
      </Button>
      {withdraw.error && (
        <ErrorLine inline>{problemMessageOf(withdraw.error, t)}</ErrorLine>
      )}
    </div>
  );
}

function OfferForm({
  kind,
  id,
}: Readonly<{ kind: TransferableRecordType; id: string }>) {
  const t = useT();
  const toast = useToast();
  const [opened, setOpened] = useState(false);
  const [toId, setToId] = useState("");
  const [note, setNote] = useState("");
  const { offer } = useOwnershipOfferActions();
  const candidates = useOwnershipOfferCandidates(kind, id, opened);
  const options = (candidates.data ?? []).map((candidate) => ({
    value: candidate.id,
    label: candidate.display_name,
  }));
  if (!opened) {
    return (
      <Button variant="link" onClick={() => setOpened(true)}>
        {t("handover.offer")}
      </Button>
    );
  }
  const send = () => {
    const name = options.find((option) => option.value === toId)?.label ?? "";
    offer.mutate(
      { recordType: kind, id, toId, note },
      { onSuccess: () => toast.show(t("handover.offerSent", { name })) },
    );
  };
  return (
    <div>
      <Field label={t("handover.offerTo")}>
        {(control) => (
          <Select
            {...control}
            options={options}
            value={toId}
            placeholder={t("handover.offerPick")}
            onChange={setToId}
          />
        )}
      </Field>
      {candidates.error && (
        <ErrorLine inline>{problemMessageOf(candidates.error, t)}</ErrorLine>
      )}
      <Field label={t("handover.offerNote")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            maxLength={NOTE_MAX}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
      <Button
        variant="primary"
        disabled={toId === ""}
        pending={offer.isPending}
        onClick={send}
      >
        {t("handover.offerSend")}
      </Button>
      <Button variant="link" onClick={() => setOpened(false)}>
        {t("handover.offerCancel")}
      </Button>
      {offer.error && (
        <ErrorLine inline>{problemMessageOf(offer.error, t)}</ErrorLine>
      )}
    </div>
  );
}
