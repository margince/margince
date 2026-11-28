// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useCallback, useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { QueryStates } from "../../design-system/querystates";
import { RecordPicker } from "../../design-system/recordpicker";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { recordRead } from "../../lib/recordread";
import { useActivityGate, writableTarget } from "../activitygate";
import { EntityRef } from "../entityref";
import { LogActivityForm } from "../logactivity";
import type { MessengerScope } from "./messenger.queries";

export function MessengerOutcome({
  scope,
}: Readonly<{ scope: MessengerScope }>) {
  const t = useT();
  const [open, Open] = useState(false);
  const title = useId();
  const gate = useActivityGate(scope.record_type);
  return (
    <>
      <Button
        variant="ghost"
        disabled={gate.pending || Boolean(gate.refusalKey)}
        reasonId={gate.refusedId}
        onClick={() => Open(true)}
      >
        {t("messenger.shareOutcome")}
      </Button>
      {gate.refusalKey && <p id={gate.sentenceId}>{t(gate.refusalKey)}</p>}
      <Modal
        open={open}
        onClose={() => Open(false)}
        labelledBy={title}
        intent="drawer"
      >
        <Heading className="modal-title" id={title} size="large">
          {t("messenger.shareOutcome")}
        </Heading>
        {open && (
          <OutcomeDestination scope={scope} onSaved={() => Open(false)} />
        )}
      </Modal>
    </>
  );
}

function OutcomeDestination({
  scope,
  onSaved,
}: Readonly<{
  scope: MessengerScope;
  onSaved: () => void;
}>) {
  const t = useT();
  const [destination, Destination] = useState({
    kind: scope.record_type,
    id: scope.id,
  });
  const record = useQuery(recordRead(destination.kind, destination.id));
  const gate = useActivityGate(
    destination.kind,
    record.isPending ? null : writableTarget(record.data ?? {}),
  );
  const [choosing, Choosing] = useState(false);
  const [writing, Writing] = useState(false);
  const search = useCallback(async (q: string) => {
    const page = unwrap(
      await api.GET("/search", {
        params: {
          query: { q, types: ["company", "deal", "project"], limit: 20 },
        },
      }),
    );
    return page.data.flatMap((hit) =>
      hit.title &&
      (hit.type === "company" || hit.type === "deal" || hit.type === "project")
        ? [{ id: `${hit.type}:${hit.id}`, name: hit.title }]
        : [],
    );
  }, []);
  if (choosing)
    return (
      <RecordPicker
        label={t("messenger.outcomeDestination")}
        searchTargets={search}
        onPick={(pick) => {
          const [kind, id] = pick.id.split(":");
          if (
            id &&
            (kind === "company" || kind === "deal" || kind === "project")
          ) {
            Destination({ kind, id });
            Choosing(false);
          }
        }}
      />
    );
  return (
    <div className="form-stack">
      <p>{t("messenger.outcomePrivacy")}</p>
      <div className="form-actions">
        <EntityRef kind={destination.kind} id={destination.id} />
        {!writing && (
          <Button variant="ghost" onClick={() => Choosing(true)}>
            {t("messenger.outcomeDestination")}
          </Button>
        )}
      </div>
      <p>{t("messenger.outcomeAudience")}</p>
      <QueryStates query={record} pendingLabel={t("messenger.loading")}>
        {writing && !gate.pending && !gate.refusalKey && !record.isError ? (
          <LogActivityForm
            key={`${destination.kind}:${destination.id}`}
            entityType={destination.kind}
            entityId={destination.id}
            askedKind="note"
            onlyNotes
            onLogged={onSaved}
          />
        ) : (
          <Button
            disabled={
              gate.pending || Boolean(gate.refusalKey) || record.isError
            }
            reasonId={gate.refusedId}
            onClick={() => Writing(true)}
          >
            {t("messenger.writeOutcome")}
          </Button>
        )}
        {gate.refusalKey && <p id={gate.sentenceId}>{t(gate.refusalKey)}</p>}
      </QueryStates>
    </div>
  );
}
