// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { IdentityLine } from "../../design-system/identityline";
import { Panel, PanelBody } from "../../design-system/panel";
import { RecordView } from "../../design-system/recordview";
import { useT } from "../../i18n";
import { invalidateRecord } from "../../lib/recordwritekeys";
import { useRecordZone } from "../../lib/recordzone";
import { RecordHistoryTab } from "../records/history";
import { RecordAccess } from "../records/recordaccess";
import { WorklistReturnLink } from "../worklist/worklist.return";
import { ContactActions } from "./contactactions";
import { ContactFacts } from "./contactfacts";
import "./contact360.css";

type Contact = components["schemas"]["Contact"];

// Every verb points at the band's sentence, so none of these is ever called.
const refused = () => undefined;

// Drawn from the row alone, because the server refuses an archived contact's
// 360. History stays: it is the one place the archive is undone.
export function ArchivedContactPage({
  contact,
  band,
  refusedReasonId,
}: Readonly<{
  contact: Contact;
  band: ReactNode;
  // Required: without it every verb below would take the press.
  refusedReasonId: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const recordZone = useRecordZone();
  return (
    <div className="wrap">
      <RecordView
        back={<WorklistReturnLink />}
        name={contact.full_name}
        identity={contact.id}
        pulse={
          <IdentityLine separator="space">
            <Badge tone="warning">{t("record.archived")}</Badge>
            <RecordAccess key={contact.id} kind="contact" record={contact} />
          </IdentityLine>
        }
        badges={<ContactFacts contact={contact} />}
        actions={
          <ContactActions
            view={{ contact }}
            contactId={contact.id}
            onWrite={refused}
            onResearch={refused}
            onLogActivity={refused}
            onAddTask={refused}
            refusedReasonId={refusedReasonId}
          />
        }
        zone={recordZone}
        band={band}
      >
        <Panel title={t("record.fullHistory")}>
          <PanelBody>
            <RecordHistoryTab
              kind="contact"
              id={contact.id}
              restore={{
                version: contact.version,
                onRestored: () =>
                  invalidateRecord(queryClient, "contact", contact.id),
              }}
            />
          </PanelBody>
        </Panel>
      </RecordView>
    </div>
  );
}
