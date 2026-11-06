// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { WriteRefused } from "../design-system/querystates";
import { INTL_LOCALE } from "../format/intllocale";
import { useLocale, useT } from "../i18n";
import { usePassports } from "../lib/passports.queries";
import { unwrap } from "../lib/problem";
import {
  areasComplete,
  effectiveAreas,
  type PassportAreas,
  type WorkArea,
  WorkAreasChoice,
  workAreaName,
} from "./workareas";

export type AreasSubject = Readonly<{
  id: string;
  name: string;
  areas: PassportAreas;
}>;

type AreasChange = Readonly<{
  gains: WorkArea[];
  loses: WorkArea[];
  changed: boolean;
}>;

// What saving would add and take away. A chosen area the role no longer
// reaches cannot be kept, since the server refuses it, so it counts as lost. A
// switch between all and custom over the same areas still changes the later
// areas the passport gets. It counts as a change with nothing gained or lost.
export function areasChange(
  before: PassportAreas,
  after: PassportAreas,
  reachable: readonly WorkArea[],
): AreasChange {
  const was = effectiveAreas(before, reachable);
  const will = effectiveAreas(after, reachable);
  const sameCustom =
    before.areas.length === after.areas.length &&
    before.areas.every((area) => after.areas.includes(area));
  const dormant =
    before.mode === "custom"
      ? before.areas.filter(
          (area) => !reachable.includes(area) && !after.areas.includes(area),
        )
      : [];
  return {
    gains: will.filter((area) => !was.includes(area)),
    loses: [...was.filter((area) => !will.includes(area)), ...dormant],
    changed:
      before.mode !== after.mode || (after.mode === "custom" && !sameCustom),
  };
}

// The area the refusal sample names: the first one the draft turns off, else
// the first one already off. None when every area is on.
function sampleRefusedArea(
  change: AreasChange,
  after: PassportAreas,
  reachable: readonly WorkArea[],
): WorkArea | undefined {
  const on = effectiveAreas(after, reachable);
  return change.loses[0] ?? reachable.find((area) => !on.includes(area));
}

/**
 * Changing where a passport may work, in a drawer. It says what the passport
 * gains and loses before the PUT, and what an agent is told in an area that is
 * off. Open it with a fresh `key` per passport, so the draft starts from that one.
 */
export function PassportAreasDrawer({
  subject,
  reachable,
  onClose,
}: Readonly<{
  subject: AreasSubject;
  reachable: readonly WorkArea[];
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const titleId = useId();
  const list = usePassports();
  const [draft, setDraft] = useState<PassportAreas>(subject.areas);
  const save = useMutation({
    mutationFn: async (request: { id: string; areas: PassportAreas }) =>
      unwrap(
        await api.PUT("/passports/{id}/areas", {
          params: { path: { id: request.id } },
          body: request.areas,
        }),
      ),
    onSuccess: async () => {
      await list.refetch();
      onClose();
    },
  });
  const change = areasChange(subject.areas, draft, reachable);
  const refused = sampleRefusedArea(change, draft, reachable);
  const names = (areas: readonly WorkArea[]) =>
    new Intl.ListFormat(INTL_LOCALE[locale], {
      style: "long",
      type: "conjunction",
    }).format(areas.map((area) => workAreaName(t, area)));
  const reason = !change.changed
    ? t("passportAreas.unchanged")
    : !areasComplete(draft)
      ? t("workArea.pickOne")
      : undefined;
  return (
    <Modal
      open
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer"
      closeDisabled={save.isPending}
      unsaved={change.changed}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {t("passportAreas.open")}
        </Heading>
        <p>{t("passportAreas.for", { name: subject.name })}</p>
      </DrawerHead>
      <DrawerBody className="form-stack">
        <WorkAreasChoice
          reachable={reachable}
          value={draft}
          onChange={setDraft}
          disabled={save.isPending}
        />
        {change.loses.length > 0 && (
          <Callout
            tone="warning"
            kind="standing"
            title={t("passportAreas.loses", { areas: names(change.loses) })}
          />
        )}
        {subject.areas.mode === "all" && draft.mode === "custom" && (
          <Callout kind="standing" title={t("passportAreas.closesLater")} />
        )}
        {change.gains.length > 0 && (
          <Callout
            kind="standing"
            title={t("passportAreas.gains", { areas: names(change.gains) })}
          />
        )}
        {refused && (
          <Callout kind="standing" title={t("passportAreas.refusalTitle")}>
            <p>
              {t("passportAreas.refusal", { area: workAreaName(t, refused) })}
            </p>
          </Callout>
        )}
        <WriteRefused titleKey="passportAreas.failed" error={save.error} />
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose} disabled={save.isPending}>
          {t("passportAreas.cancel")}
        </Button>
        <Button
          variant="primary"
          reason={save.isPending ? undefined : reason}
          pending={save.isPending}
          busyLabel={t("passportAreas.saving")}
          onClick={() => save.mutate({ id: subject.id, areas: draft })}
        >
          {t("passportAreas.save")}
        </Button>
      </DrawerFoot>
    </Modal>
  );
}
