// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import { OvernightGrantChoice } from "../../../components/overnight-grant-choice";
import { ActionRow } from "../../../design-system/actionrow";
import {
  Badge,
  Button,
  Field,
  Modal,
  TextInput,
} from "../../../design-system/atoms";
import { ErrorLine } from "../../../design-system/errorline";
import { Heading } from "../../../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../../../design-system/panel";
import { SettingList, SettingRow } from "../../../design-system/settingrow";
import { Row } from "../../../design-system/stack";
import { useT } from "../../../i18n";

// What the connect step offers beside the mailbox, in either state: the
// overnight brief and the LinkedIn profile. Neither gates the step.
//
// LinkedIn is not a connection here. Nothing is authorized and nothing syncs.
// The member records which profile their imported network is attributed to.
// A connection they bring in later then reads "Anna knows them" rather than
// "the company knows them". The import itself lives in Settings.

export type LinkedinStatus = "pending" | "saved" | "skipped";

export type AlsoWorthDoingProps = Readonly<{
  /** The rep's preselected answer to the overnight question. Owned by the act,
   *  because that is where it rides along with the step's continue. */
  wantsOvernight: boolean;
  onWantsOvernightChange: (next: boolean) => void;
  /** The answer could not be recorded. The step completed anyway, so this says
   *  so rather than blocking: the question is askable again in Settings. */
  overnightFailed: boolean;
  linkedinStatus: LinkedinStatus;
  onLinkedinSave: (profileUrl: string) => void;
  onLinkedinSkip: () => void;
  linkedinPending: boolean;
  linkedinError: string | null;
}>;

export function AlsoWorthDoing(props: AlsoWorthDoingProps) {
  const t = useT();
  return (
    <Panel title={t("ob.conv.connect.alsoTitle")}>
      <PanelBody>
        <PanelIntro>{t("ob.conv.connect.alsoSub")}</PanelIntro>
        <SettingList>
          {/* Asked on the step rather than in the ask, because a real OAuth
              allow leaves the page entirely: a switch flipped inside the ask
              would not survive the redirect back. */}
          <OvernightGrantChoice
            checked={props.wantsOvernight}
            onChange={props.onWantsOvernightChange}
            failed={props.overnightFailed}
          />
          <LinkedinRow {...props} />
        </SettingList>
      </PanelBody>
    </Panel>
  );
}

function LinkedinRow({
  linkedinStatus: status,
  onLinkedinSave,
  onLinkedinSkip,
  linkedinPending: pending,
  linkedinError: error,
}: AlsoWorthDoingProps) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const titleId = useId();
  const description = {
    pending: t("ob.conv.linkedin.cardBody"),
    saved: t("ob.conv.connect.linkedinSaved"),
    skipped: t("ob.conv.connect.linkedinSkippedNote"),
  }[status];

  return (
    <>
      <SettingRow
        label={
          // The default tone: LinkedIn never gates the act.
          <Row>
            {t("ob.conv.connect.linkedinRow")}
            <Badge>{t("ob.conv.connect.recommended")}</Badge>
          </Row>
        }
        description={description}
        control={
          status === "pending" ? (
            <Button variant="ghost" onClick={() => setOpen(true)}>
              {t("ob.conv.connect.linkedinAdd")}
            </Button>
          ) : (
            status === "saved" && (
              <Badge tone="success">{t("ob.conv.connect.savedCta")}</Badge>
            )
          )
        }
      />
      <Modal
        open={open && status === "pending"}
        // X, Escape and the backdrop all resolve to this one handler. Guarding it here
        // keeps every dismissal from racing the save: a PUT landing after the reader
        // backed out would save against a closed ask.
        onClose={() => {
          if (!pending) {
            setOpen(false);
          }
        }}
        labelledBy={titleId}
        intent="form"
      >
        <Heading size="large" className="t-h2 modal-title" id={titleId}>
          {t("ob.conv.linkedin.dialogHeadline")}
        </Heading>
        <LinkedinPanel
          // A failed save stays on screen so `error` is seen and retried; a
          // successful one closes the dialog by flipping `status` to "saved".
          onSave={onLinkedinSave}
          onSkip={() => {
            onLinkedinSkip();
            setOpen(false);
          }}
          pending={pending}
          error={error}
        />
      </Modal>
    </>
  );
}

// The profile ask: one URL and why it is wanted. No scope list, because
// nothing is being granted. The address is written to the member's own account
// and read only by the import that attributes their network.
function LinkedinPanel({
  onSave,
  onSkip,
  pending,
  error,
}: Readonly<{
  onSave: (profileUrl: string) => void;
  onSkip: () => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  const [profile, setProfile] = useState("");
  const trimmed = profile.trim();

  return (
    <>
      <div className="form-stack">
        <Field label={t("ob.conv.linkedin.profileLabel")}>
          {(control) => (
            <TextInput
              {...control}
              type="url"
              inputMode="url"
              placeholder={t("ob.conv.linkedin.profilePlaceholder")}
              value={profile}
              onChange={(event) => setProfile(event.target.value)}
            />
          )}
        </Field>
        <p className="t-sub">{t("ob.conv.linkedin.profileWhy")}</p>
        {error !== null && <ErrorLine>{error}</ErrorLine>}
        <p className="t-sub">{t("ob.conv.linkedin.importLater")}</p>
      </div>
      <ActionRow
        className="actions"
        primary={
          <Button
            variant="primary"
            pending={pending}
            disabled={trimmed === ""}
            onClick={() => onSave(trimmed)}
          >
            {t("ob.conv.linkedin.save")}
          </Button>
        }
      >
        {/* Skipping and saving answer the same question, so they cannot both
            be in flight: a skip landing mid-save leaves two answers stored. */}
        <Button variant="ghost" disabled={pending} onClick={onSkip}>
          {t("ob.conv.linkedin.skip")}
        </Button>
      </ActionRow>
    </>
  );
}
