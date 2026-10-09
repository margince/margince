// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Button, EmptyState, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { captureValueMessage } from "./capturevalue";
import { problemMessageOf, QueryGate, throwProblem, unwrap } from "./common";

// The own-domain surface (CAP-WIRE-2a, ADR-0082/A127): which domains this
// installation treats as its own, and therefore whose mail it does not store.
// Every role reads it — a rep should be able to see why a thread is missing —
// and only admin/ops may change it, so the verbs are refused rather than
// hidden, like the capture-settings card beside it.
//
// One card with two named rows, because the two lists answer the same question
// — which domains are ours — and differ only in who owns the answer: the
// company profile claims the first set and this screen cannot touch them, the
// second is curated here. As two cards they read as two subjects; as two rows
// with their own labels the difference in ownership is the thing the reader
// sees, which is what it actually is.

// The row shape comes from the generated contract rather than being restated
// here: a hand-written copy would drift the first time the contract gains a
// field, and drift silently, since nothing compares the two.
type WorkspaceEmailDomain = components["schemas"]["WorkspaceEmailDomain"];

function useOwnDomains() {
  return useQuery({
    queryKey: ["workspace-email-domains"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/email-domains");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useAddOwnDomain() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (domain: string) => {
      return unwrap(
        await api.POST("/capture/email-domains", {
          body: { domain },
        }),
      );
    },
    onSuccess: (_written, domain) => {
      queryClient.invalidateQueries({ queryKey: ["workspace-email-domains"] });
      toast.show(t("settings.addedItem", { name: domain }));
    },
  });
}

function useRemoveOwnDomain() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (domain: string) => {
      unwrap(
        await api.DELETE("/capture/email-domains/{domain}", {
          params: { path: { domain } },
        }),
      );
    },
    onSuccess: (_written, domain) => {
      queryClient.invalidateQueries({ queryKey: ["workspace-email-domains"] });
      toast.show(t("settings.removedItem", { name: domain }));
    },
  });
}

export function OwnDomainsCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useOwnDomains();
  const remove = useRemoveOwnDomain();
  const [adding, setAdding] = useState(false);
  // The denial, said once and POINTED AT. A control that is refused with its
  // reason floating somewhere further down the card has told a screen reader
  // nothing: the reason is read only if the reader happens to arrive at that
  // paragraph, which is not where the refused control left them. `Button`'s
  // `reasonId` is that wiring — it refuses the control AND names the one
  // sentence already on the page — so several refused verbs say it once. The
  // id is minted unconditionally, because a hook may not depend on a
  // permission.
  const denialId = useId();
  const refusal = canManage ? undefined : denialId;
  // One request feeds both groups. The anchors stay out of the gate: a list
  // nobody may edit has nothing to say while the read is in flight.
  const anchors = query.data?.anchor_domains ?? [];

  // Panel rather than Card, and no per-card bottom margin: the settings page
  // owns the gap between its surfaces in one place, so a card cannot space
  // itself differently from the one beside it.
  return (
    <Panel
      title={t("ownDomains.title")}
      // A create form behind one verb, so the rows below stay answers — and the
      // verb rides in the header rather than in a row of its own, because a row
      // states a setting and its answer while a row whose label repeats its own
      // button says the same thing twice a hand apart. Refused, never hidden:
      // `reasonId` names the one sentence under the rows.
      titleAction={
        <Button reasonId={refusal} onClick={() => setAdding(true)}>
          {t("ownDomains.addOpen")}
        </Button>
      }
    >
      <PanelBody>
        <PanelIntro>{t("ownDomains.sub")}</PanelIntro>
      </PanelBody>
      {anchors.length > 0 && (
        <>
          <PanelGroupHead title={t("ownDomains.companyTitle")} level="h3" />
          <PanelBody>
            {/* Copy that sends the reader somewhere has to take them there:
                these rows cannot be edited here, only on the company profile. */}
            <PanelIntro>
              {t("ownDomains.fromCompany")}{" "}
              <a href="#/settings/company">{t("ownDomains.openCompany")}</a>
            </PanelIntro>
          </PanelBody>
          <SettingList bleed="records" testId="own-domains-from-company">
            {anchors.map((domain) => (
              <SettingRow key={domain} label={domain} control={null} />
            ))}
          </SettingList>
        </>
      )}
      <PanelGroupHead title={t("ownDomains.curatedTitle")} level="h3" />
      <PanelBody>
        <PanelIntro>{t("ownDomains.irreversible")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("ownDomains.curatedTitle")}>
        {(list) => (
          <CuratedDomains
            list={list.data}
            refusal={refusal}
            pending={remove.isPending}
            onRemove={(domain) => remove.mutate(domain)}
          />
        )}
      </QueryGate>
      {(!canManage || remove.isError) && (
        <PanelBody className="form-stack">
          {!canManage && <p id={denialId}>{t("captureSettings.adminOnly")}</p>}
          {remove.isError && (
            <Callout
              kind="outcome"
              tone="danger"
              title={t("ownDomains.removeFailed")}
            >
              {problemMessageOf(remove.error, t)}
            </Callout>
          )}
        </PanelBody>
      )}
      {adding && <AddOwnDomainDialog onClose={() => setAdding(false)} />}
    </Panel>
  );
}

/**
 * The curated half: one row per domain, whether it is confirmed as the row's
 * answer, and the verb that takes it back at the right.
 *
 * It was a hand-rolled `<ul>` with an inline-styled `<li>` and an inline
 * `marginLeft` on the state beside each domain — the same list
 * capture-exclusions.tsx had copied, which is the signal the shape belongs to
 * the row language rather than to either screen.
 */
function CuratedDomains({
  list,
  refusal,
  pending,
  onRemove,
}: Readonly<{
  list: WorkspaceEmailDomain[];
  /** The id of the one sentence saying why removal is refused, when it is. */
  refusal: string | undefined;
  pending: boolean;
  onRemove: (domain: string) => void;
}>) {
  const t = useT();
  if (list.length === 0) {
    // `empty`, and only `empty`: no further domain is registered, which is a
    // fact about the installation rather than a read that failed.
    return (
      <EmptyState>
        <p data-testid="own-domains-empty">{t("ownDomains.empty")}</p>
      </EmptyState>
    );
  }
  return (
    <SettingList bleed="records" testId="own-domains-list">
      {list.map((domain) => (
        <SettingRow
          key={domain.domain}
          label={domain.domain}
          value={
            domain.verified
              ? t("ownDomains.confirmed")
              : t("ownDomains.candidate")
          }
          control={
            <IconAction
              variant="ghost"
              label={t("ownDomains.remove", { domain: domain.domain })}
              icon={<Trash2 aria-hidden />}
              disabled={pending}
              reasonId={refusal}
              onClick={() => onRemove(domain.domain)}
            />
          }
        />
      ))}
    </SettingList>
  );
}

// Mounted only while it is open, so the draft a reader abandoned is gone the
// next time they open it rather than waiting there as a half-typed domain.
function AddOwnDomainDialog({ onClose }: Readonly<{ onClose: () => void }>) {
  const t = useT();
  const add = useAddOwnDomain();
  const headingId = useId();
  const formId = useId();
  const [draft, setDraft] = useState("");
  const domain = draft.trim();
  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("ownDomains.addLabel")}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (domain === "") {
            return;
          }
          add.mutate(domain, { onSuccess: onClose });
        }}
      >
        <TextInput
          value={draft}
          aria-label={t("ownDomains.addLabel")}
          placeholder={t("ownDomains.placeholder")}
          onChange={(event) => setDraft(event.target.value)}
        />
        {add.isError && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("ownDomains.addFailed")}
          >
            {captureValueMessage(add.error, "domain", t)}
          </Callout>
        )}
      </form>
      <div className="actions">
        <Button type="button" onClick={onClose}>
          {t("create.cancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={add.isPending || domain === ""}
        >
          {t("ownDomains.add")}
        </Button>
      </div>
    </Modal>
  );
}
