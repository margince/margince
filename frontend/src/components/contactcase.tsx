// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { problemStatus } from "../lib/problem";
import { CaseForm } from "./contactcase.form";
import {
  type ContactCase,
  type ContactCaseOutcome,
  useContactCase,
} from "./contactcase.queries";
import { EntityRef } from "./entityref";
import "./contactcase.css";

/**
 * The one review of a contact duplicate case, opened from the Worklist row and
 * from the contact page. `contactId` is any member; the review starts from it.
 */
export function ContactCaseReview({
  contactId,
  open,
  onClose,
}: Readonly<{ contactId: string; open: boolean; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("contactCase.title")}
        </Heading>
      </DrawerHead>
      {open && <CaseReviewBody contactId={contactId} onClose={onClose} />}
    </Modal>
  );
}

function CaseReviewBody({
  contactId,
  onClose,
}: Readonly<{ contactId: string; onClose: () => void }>) {
  const t = useT();
  const query = useContactCase(contactId);
  const [stale, setStale] = useState(false);
  const [merged, setMerged] = useState<ContactCaseOutcome | null>(null);
  const [seenOpen, setSeenOpen] = useState(false);
  const [mergedPair, setMergedPair] = useState(false);
  const contactCase = query.data;
  if (contactCase && !seenOpen) setSeenOpen(true);
  if (merged) {
    return <MergedBody outcome={merged} onClose={onClose} />;
  }
  // The case vanished under an open review, and no pair was merged here.
  // So a colleague settled it, and this contact is the one that remained.
  const settledElsewhere = contactCase === null && seenOpen && !mergedPair;
  if (!contactCase || contactCase.status === "resolved") {
    return (
      <>
        <DrawerBody>
          <SurfaceState
            state={readState(query, contactCase)}
            loadingLabel={t("contactCase.loading")}
            emptyLabel={t(
              settledElsewhere
                ? "contactCase.settledElsewhere"
                : "contactCase.none",
            )}
            detail={{
              withheldReason: t("contactCase.withheld"),
              onRetry: () => void query.refetch(),
            }}
          >
            {contactCase && <ResolvedNote contactCase={contactCase} />}
          </SurfaceState>
        </DrawerBody>
        <DrawerFoot className="actions">
          <Button onClick={onClose}>{t("common.close")}</Button>
        </DrawerFoot>
      </>
    );
  }
  return (
    <CaseForm
      key={contactCase.revision}
      contactCase={contactCase}
      startId={contactId}
      stale={stale}
      onClose={onClose}
      onStale={() => {
        setStale(true);
        void query.refetch();
      }}
      onMerged={setMerged}
      onPairMerged={() => setMergedPair(true)}
    />
  );
}

function readState(
  query: ReturnType<typeof useContactCase>,
  contactCase: ContactCase | null | undefined,
) {
  if (contactCase) return "ready";
  if (contactCase === null) return "empty";
  if (query.isPending) return "loading";
  return problemStatus(query.error) === 403 ? "withheld" : "failed";
}

function ResolvedNote({ contactCase }: Readonly<{ contactCase: ContactCase }>) {
  const t = useT();
  const resolution = contactCase.resolution;
  return (
    <Callout
      tone="info"
      title={
        resolution?.resolved_by_name
          ? t("contactCase.resolvedBy", { name: resolution.resolved_by_name })
          : t("contactCase.resolved")
      }
    >
      {resolution && (
        <p>
          {t("contactCase.resolvedInto")}{" "}
          <EntityRef kind="contact" id={resolution.survivor_id} />
        </p>
      )}
    </Callout>
  );
}

function MergedBody({
  outcome,
  onClose,
}: Readonly<{ outcome: ContactCaseOutcome; onClose: () => void }>) {
  const t = useT();
  return (
    <>
      <DrawerBody>
        <Callout tone="success" title={t("contactCase.merged")}>
          <p>
            {t("contactCase.resolvedInto")}{" "}
            <EntityRef
              kind="contact"
              id={outcome.survivor_id}
              name={outcome.survivor?.full_name}
            />
          </p>
        </Callout>
      </DrawerBody>
      <DrawerFoot className="actions">
        <Button onClick={onClose}>{t("common.close")}</Button>
      </DrawerFoot>
    </>
  );
}
