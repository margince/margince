// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useCaptureSettings } from "../../components/capture-settings";
import { Button, Checkbox, Disclosure } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { useSaveDraft } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { throwProblem } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { ConnectorContextTagRow } from "./connectors.contexttag";
import { HistoryBaselineRow } from "./connectors.historybaseline";
import { useHeldThreads } from "./held-threads";

// The standing decisions about one connected mailbox, drawn under its account
// on Connections. They cover who may read what it brings in, where its held
// threads are reviewed, and the rarer answers behind Advanced.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type MailPosture = NonNullable<CaptureConnection["mail_posture"]>;

/**
 * The settings block for an account. A connected mailbox gets the whole block.
 * A service without mail (a calendar, the test mailbox) keeps only the tag the
 * contacts it brings in are filed under.
 */
export function AccountSettings({
  mailbox,
  others,
  accountName,
}: Readonly<{
  /** The account's connected mailbox, or null. */
  mailbox: CaptureConnection | null;
  /** The account's other connected services, which carry no mail. */
  others: readonly CaptureConnection[];
  accountName: string;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const otherTags = others.map((conn) => (
    <ConnectorContextTagRow key={conn.provider} conn={conn} />
  ));
  if (!mailbox) {
    return others.length > 0 ? <SettingList>{otherTags}</SettingList> : null;
  }
  // Grouped by service once there are two, because the tag is one answer per
  // connection and two rows of one name would read as one.
  const grouped = others.length > 0;
  // The signature switch, the history baseline and the mailbox's tag, plus one
  // tag per other service.
  const advancedCount = 3 + others.length;
  return (
    <div className="connection-settings">
      <Heading size="xsmall" as="h4">
        {t("connectors.accountSettings", { account: accountName })}
      </Heading>
      <SettingList>
        <MailPostureRow conn={mailbox} />
        <HeldPointerRow />
      </SettingList>
      <Disclosure
        summary={
          <>
            {t("connectors.advanced")}{" "}
            <span className="t-caption">
              {plural("connectors.advancedCount", advancedCount, {
                count: formatNumber(advancedCount, locale),
              })}
            </span>
          </>
        }
      >
        {grouped && <p className="t-caption">{t("connectors.bringsMail")}</p>}
        <SettingList>
          <SignatureEnrichmentRow conn={mailbox} />
          <HistoryBaselineRow conn={mailbox} />
          <ConnectorContextTagRow conn={mailbox} />
        </SettingList>
        {grouped && (
          <>
            <p className="t-caption">{t("connectors.bringsCalendar")}</p>
            <SettingList>{otherTags}</SettingList>
          </>
        )}
      </Disclosure>
    </div>
  );
}

// The held threads are reviewed on Capture activity. The count is the
// reader's whole set: the list is one per member, not one per mailbox.
function HeldPointerRow() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const held = useHeldThreads();
  const count = held.data?.data.length ?? null;
  let description = t("connectors.heldPointer.unknown");
  if (count === 0) {
    description = t("connectors.heldPointer.none");
  } else if (count !== null) {
    description = plural("connectors.heldPointer.some", count, {
      count: formatNumber(count, locale),
    });
  }
  return (
    <SettingRow
      testId="connector-held-pointer"
      label={t("connectors.heldPointer.label")}
      description={description}
      control={
        <Button
          variant="link"
          onClick={() => navigate(settingsHref("capture-activity"))}
        >
          {t("connectors.heldPointer.review")}
          <ArrowRight aria-hidden="true" />
        </Button>
      }
    />
  );
}

// What this mailbox asks of the mail it brings in. A Select rather than a
// Switch, because the three answers are not one thing turned on and off.
// `held` and `classified` both hold a message to the contacts on it, and differ
// in whether a classifier may ever open it later.
//
// `shared` is present and refused rather than absent when the workspace has not
// allowed it. A reader then learns there is a third answer and who can turn it
// on. The server refuses it too (422 `shared_posture_not_allowed`).
//
// Each posture has a strictness, so the row can tell a narrowing from a
// widening. Only a narrowing has anything to offer history, because
// `apply_to_history` only ever narrows.
const postureRank: Record<MailPosture, number> = {
  shared: 0,
  classified: 1,
  held: 2,
};

const postureHelp: Record<MailPosture, MessageKey> = {
  shared: "connectors.mailPosture.help.shared",
  classified: "connectors.mailPosture.help.classified",
  held: "connectors.mailPosture.help.held",
};

function isMailPosture(value: string): value is MailPosture {
  return Object.hasOwn(postureRank, value);
}

function MailPostureRow({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetMailPosture(conn.provider);
  const [draft, setDraft] = useState<MailPosture | null>(null);
  const [applyToHistory, setApplyToHistory] = useState(false);
  const sharedAllowed = settings.data?.shared_posture_allowed ?? false;
  const posture = conn.mail_posture ?? "classified";
  const shown = draft ?? posture;
  // Only a narrowing asks about history; a widening sends false whatever was ticked.
  const narrowing = postureRank[shown] > postureRank[posture];
  const discard = () => {
    save.reset();
    setDraft(null);
    setApplyToHistory(false);
  };
  useSaveDraft({
    dirty: shown !== posture,
    save: async () => {
      await save.mutateAsync({
        posture: shown,
        applyToHistory: narrowing && applyToHistory,
      });
      setDraft(null);
      setApplyToHistory(false);
    },
    discard,
  });
  const help = t(postureHelp[shown]);

  return (
    <>
      <SettingRow
        testId={`connector-${conn.provider}-mail-posture`}
        label={t("connectors.mailPosture.label")}
        // The reason a refused answer is greyed rides in the description: a
        // listbox row ellipsises, and the cut half is the half a reader needs.
        description={
          sharedAllowed
            ? help
            : `${help} ${t("connectors.mailPosture.sharedNeedsAdmin")}`
        }
        control={(controlProps) => (
          <Select
            {...controlProps}
            value={shown}
            disabled={save.isPending}
            onChange={(next) => {
              if (isMailPosture(next)) setDraft(next);
            }}
            options={[
              {
                value: "classified",
                label: t("connectors.mailPosture.classified"),
              },
              { value: "held", label: t("connectors.mailPosture.held") },
              {
                value: "shared",
                label: t("connectors.mailPosture.shared"),
                disabled: !sharedAllowed,
              },
            ]}
          />
        )}
      />
      {narrowing && (
        <SettingRow
          testId={`connector-${conn.provider}-mail-posture-history`}
          label={t("connectors.mailPosture.historyTitle")}
          description={t("connectors.mailPosture.historyBody")}
          control={
            <Checkbox
              checked={applyToHistory}
              disabled={save.isPending}
              onChange={(e) => setApplyToHistory(e.target.checked)}
              label={t("connectors.mailPosture.historyApply")}
            />
          }
        />
      )}
    </>
  );
}

function useSetMailPosture(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    // Both halves of the decision arrive as variables: a mutationFn closing
    // over rendered state answers with the previous render's.
    mutationFn: async (vars: {
      posture: MailPosture;
      applyToHistory: boolean;
    }) => {
      const { error } = await api.PUT("/connectors/{provider}/mail-posture", {
        params: { path: { provider } },
        body: { posture: vars.posture, apply_to_history: vars.applyToHistory },
      });
      if (error) {
        throwProblem(error);
      }
    },
    // Awaited, so the row never flashes the old posture once the draft drops.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["connectors"] }),
  });
}

// This mailbox's own answer to the nightly signature pass. Tri-state on the
// wire, two states on screen. The description says whether the answer is the
// mailbox's own or the company's, and turning the switch makes it its own.
function SignatureEnrichmentRow({
  conn,
}: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const settings = useCaptureSettings();
  const save = useSetSignatureEnrichment(conn.provider);
  const workspaceDefault = settings.data?.signature_enrich ?? true;
  const own = conn.signature_enrich_enabled;
  const effective = own ?? workspaceDefault;
  return (
    <SettingRow
      testId={`connector-${conn.provider}-signature-enrich`}
      label={t("connectors.signatureEnrich.label")}
      description={
        own === null || own === undefined
          ? t("connectors.signatureEnrich.followingDefault")
          : t("connectors.signatureEnrich.ownAnswer")
      }
      control={
        <Switch
          testId={`connector-${conn.provider}-signature-enrich-toggle`}
          label={t("connectors.signatureEnrich.label")}
          labelHidden
          checked={effective}
          disabled={save.isPending}
          onChange={(next) => save.mutate(next)}
        />
      }
    />
  );
}

function useSetSignatureEnrichment(provider: CaptureConnection["provider"]) {
  const queryClient = useQueryClient();
  return useMutation({
    // The new answer is a VARIABLE, so the press belongs to the committed render.
    mutationFn: async (enabled: boolean) => {
      const { error } = await api.PUT(
        "/connectors/{provider}/signature-enrichment",
        { params: { path: { provider } }, body: { enabled } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["connectors"] });
    },
  });
}
