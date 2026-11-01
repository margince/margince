// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useMemo, useState } from "react";
import { Button, Modal, SegmentedControl } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { NOTICE_CASE_PARAM } from "../../lib/privacycaselink";
import { useUrlParams } from "../../lib/urlstate";
import { NoticeCasesCard } from "../privacy/noticecases";
import { NewDsrForm, PrivacyInboxCard } from "../privacy/privacy";
import {
  ConfirmSubmissionsPanel,
  useCorrections,
} from "../privacy/privacy.corrections";
import { ConsentPurposesCard } from "../privacy/privacy.purposes";
import { RetentionCard } from "../privacy/retention";
import {
  PinRecordAction,
  RestrictedRecordsCard,
  useRestrictedRecords,
} from "./restrictedrecords";

/** The governance page's panels in working order: defaults, the requests
 * that override them, then the duties nobody asked for. */
export function PrivacyLanes() {
  const [params] = useUrlParams();
  const caseId = params.get(NOTICE_CASE_PARAM);
  if (caseId) return <NoticeCasesCard key={caseId} caseId={caseId} />;
  return (
    <>
      <ConsentPurposesCard />
      <RetentionCard />
      <RequestsPanel />
      <NoticeCasesCard />
    </>
  );
}

const LANES = ["requests", "corrections", "restricted"] as const;
type Lane = (typeof LANES)[number];

/** The work a contact asked for, one list at a time. One officer works all
 * three lists, so they share one panel. */
export function RequestsPanel() {
  const t = useT();
  const [lane, setLane] = useState<Lane>("requests");
  const corrections = useCorrections();
  const restricted = useRestrictedRecords(useCan("retention_policy", "read"));
  const labels = useMemo(
    (): Record<Lane, string> => ({
      requests: t("privacy.laneRequests"),
      corrections: t("privacy.laneCorrections"),
      restricted: t("privacy.laneRestricted"),
    }),
    [t],
  );
  // A paged queue with more to load knows only a floor, so it shows no count.
  const counts: Partial<Record<Lane, number>> = {};
  if (corrections.isSuccess && !corrections.hasNextPage) {
    counts.corrections = corrections.data.pages.reduce(
      (sum, page) => sum + page.data.length,
      0,
    );
  }
  if (restricted.isSuccess && !restricted.data.page.has_more) {
    counts.restricted = restricted.data.data.length;
  }

  let verb: ReactNode = null;
  if (lane === "requests") {
    verb = <NewRequestAction />;
  } else if (lane === "restricted") {
    verb = <PinRecordAction />;
  }
  return (
    <Panel title={t("privacy.requestsPanel")} titleAction={verb}>
      {/* Its own band, so the switch keeps focus while the lane under it changes. */}
      <PanelBody>
        <SegmentedControl
          options={LANES}
          value={lane}
          onChange={setLane}
          labels={labels}
          counts={counts}
          label={t("privacy.lanesLabel")}
        />
      </PanelBody>
      {lane === "requests" && <PrivacyInboxCard />}
      {lane === "corrections" && <ConfirmSubmissionsPanel />}
      {lane === "restricted" && <RestrictedRecordsCard />}
    </Panel>
  );
}

// The subject-request lane's verb, in the panel head.
function NewRequestAction() {
  const t = useT();
  const titleId = useId();
  const [creating, setCreating] = useState(false);
  // Opening a request writes the contact it names (consent/dsr.go CreateDSR).
  const canOpenRequest = useCanWrite("contact", "update");
  if (!canOpenRequest) {
    return null;
  }
  return (
    <>
      <Button aria-haspopup="dialog" onClick={() => setCreating(true)}>
        {t("privacy.newRequest")}
      </Button>
      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        labelledBy={titleId}
        intent="form"
      >
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {t("privacy.newRequest")}
        </Heading>
        <NewDsrForm onDone={() => setCreating(false)} />
      </Modal>
    </>
  );
}
