// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  Disclosure,
  Field,
  Modal,
  PendingBody,
  TextInput,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { FileDropzone } from "../../design-system/filedropzone";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Stack } from "../../design-system/stack";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { statusTone } from "../../design-system/statusbadge";
import { Switch } from "../../design-system/switch";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { postMultipart } from "../../lib/multipart";
import { problemMessageOf, unwrap } from "../../lib/problem";
import {
  LINKEDIN_ACCOUNT_KEY,
  useSaveLinkedInAccount,
} from "../../lib/use-linkedin-account";

// The LinkedIn network: the member's own Connections.csv
// export, the profile it is credited to, and the companies it reaches. No app
// approval, no OAuth. The imported rows never become contacts, and the row says
// so before the member uploads their address book into a company system.

type ImportSummary = {
  rows: number;
  imported: number;
  skipped: number;
  confirmed: number;
  suggested: number;
};

function useImportConnections() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (file: File): Promise<ImportSummary> => {
      const payload = await postMultipart("/v1/me/linkedin-connections", {
        file,
      });
      return payload as ImportSummary;
    },
    // An import changes the account count, the connections and the reach, which
    // hold separate cache keys; without this the panel contradicts itself.
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: LINKEDIN_ACCOUNT_KEY });
      await client.invalidateQueries({ queryKey: ["linkedin-connections"] });
      await client.invalidateQueries({ queryKey: ["linkedin-reach"] });
    },
  });
}

type LinkedInAccount = components["schemas"]["LinkedInAccount"];

function useLinkedInAccount() {
  return useQuery({
    queryKey: LINKEDIN_ACCOUNT_KEY,
    queryFn: async (): Promise<LinkedInAccount> => {
      return unwrap(await api.GET("/me/linkedin-account", {}));
    },
  });
}

// Your own LinkedIn profile, shown back so it can be corrected. The API has no
// path to another member's. Value and verb sit on the row, the field in a
// modal. The row says what it is set to, not only that it is editable.
function LinkedInProfileRow() {
  const t = useT();
  const account = useLinkedInAccount();
  const save = useSaveLinkedInAccount();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string | null>(null);
  const headingId = useId();
  const formId = useId();

  const stored = account.data?.profile_url ?? "";
  // Adopt the server value until the member starts typing, so a save or a
  // refetch is reflected without discarding an edit in progress. Adjusted
  // during render rather than in an effect: an effect would paint the stale
  // value first and then correct it.
  const [seen, setSeen] = useState(stored);
  if (seen !== stored) {
    setSeen(stored);
    setDraft(null);
  }
  const value = draft ?? stored;
  const dirty = value.trim() !== stored;

  // Closing is abandoning: the draft goes and so does the previous attempt's
  // refusal, which would otherwise be on screen the next time the dialog opens
  // and read as a fresh failure of an edit nobody has made yet.
  const close = () => {
    setEditing(false);
    setDraft(null);
    save.reset();
  };

  if (account.isError) {
    // The row keeps its place and says the read failed. `SettingList` rules
    // between whatever it holds, so a surface standing in for a row lines up
    // with the rows around it.
    return (
      <Callout
        kind="outcome"
        tone="danger"
        title={t("linkedinImport.profileReadFailed")}
      >
        {problemMessageOf(account.error, t)}
      </Callout>
    );
  }

  return (
    <>
      <SettingRow
        label={t("linkedinImport.editProfileTitle")}
        // An empty URL is "not added here", not "the member has no profile".
        value={stored === "" ? t("linkedinImport.profileNotSet") : stored}
        description={
          account.data?.connected
            ? t("linkedinImport.connectedNote")
            : t("linkedinImport.notConnectedNote")
        }
        control={
          <Button variant="ghost" onClick={() => setEditing(true)}>
            {stored === ""
              ? t("linkedinImport.addProfile")
              : t("linkedinImport.editProfile")}
          </Button>
        }
      />
      <Modal
        open={editing}
        onClose={close}
        labelledBy={headingId}
        intent="form"
      >
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("linkedinImport.editProfileTitle")}
        </Heading>
        <form
          id={formId}
          className="form-stack"
          onSubmit={(event) => {
            event.preventDefault();
            if (dirty) {
              // Correcting the URL is not disconnecting: the save carries
              // `connected: false` and the store keeps whatever authorization
              // the member has already given.
              save.mutate(
                { profileUrl: value.trim(), connected: false },
                { onSuccess: () => setEditing(false) },
              );
            }
          }}
        >
          <Field label={t("linkedinImport.profileLabel")}>
            {(control) => (
              <TextInput
                {...control}
                type="url"
                inputMode="url"
                data-testid="linkedin-profile-url"
                placeholder={t("linkedinImport.profilePlaceholder")}
                value={value}
                onChange={(e) => setDraft(e.target.value)}
              />
            )}
          </Field>
          {save.isError && (
            <Callout
              kind="outcome"
              tone="danger"
              title={t("linkedinImport.saveFailed")}
            >
              {problemMessageOf(save.error, t)}
            </Callout>
          )}
        </form>
        <div className="actions">
          <Button type="button" onClick={close} disabled={save.isPending}>
            {t("create.cancel")}
          </Button>
          {/* `disabled` for an unchanged URL the reader fixes by typing;
              `pending` keeps a started save focusable so its wait is announced. */}
          <Button
            variant="primary"
            type="submit"
            form={formId}
            disabled={!dirty}
            pending={save.isPending}
          >
            {t("linkedinImport.saveProfile")}
          </Button>
        </div>
      </Modal>
    </>
  );
}

// Whether colleagues who may read a company see how many of this member's
// connections work there.
//
// On by default, and the description says so, because a member who never opens
// this is listed. The count is a derived fact and names no connection, which is
// what makes a default-on setting defensible. It is still a fact about them, so
// they may withhold it.
//
// Their own reach page is unaffected either way: that one answers about
// themselves, and this governs what colleagues are shown.
function ReachVisibilityRow() {
  const t = useT();
  const account = useLinkedInAccount();
  const client = useQueryClient();
  const save = useMutation({
    mutationFn: async (visible: boolean): Promise<void> => {
      unwrap(
        await api.PUT("/me/linkedin-account", {
          body: { reach_visible_to_team: visible },
        }),
      );
    },
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: LINKEDIN_ACCOUNT_KEY });
      // The colleague lists every company page draws from this answer.
      await client.invalidateQueries({ queryKey: ["company-linkedin-reach"] });
    },
  });
  // Only a successful read answers this. Until then the control is unavailable
  // rather than showing the default. A member who withheld their reach would
  // see it on, and a click would write a state nobody read back.
  const answered = account.isSuccess;
  const visible = account.data?.reach_visible_to_team ?? true;
  return (
    <SettingRow
      label={t("linkedinImport.reachVisibleLabel")}
      description={t("linkedinImport.reachVisibleHelp")}
      control={(props) => (
        <Stack gap="2">
          <Switch
            label={t("linkedinImport.reachVisibleLabel")}
            labelHidden
            checked={answered && visible}
            reason={
              answered ? undefined : t("linkedinImport.reachVisibleUnread")
            }
            pending={save.isPending}
            describedBy={props["aria-describedby"]}
            onChange={(next) => save.mutate(next)}
          />
          {save.isError && (
            <Callout
              kind="outcome"
              tone="danger"
              title={t("linkedinImport.reachVisibleSaveFailed")}
            >
              {problemMessageOf(save.error, t)}
            </Callout>
          )}
        </Stack>
      )}
    />
  );
}

/**
 * The LinkedIn network: the import as a connection row, the profile it is
 * credited to, the companies it reaches, and where to get the file.
 */
export function LinkedInNetworkPanel() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const account = useLinkedInAccount();
  const held = account.data?.connections;
  return (
    <Panel title={t("linkedinReach.title")}>
      <PanelBody>
        <ProviderList>
          <ProviderRow
            mark={<ProviderMark providerKey="linkedin" />}
            name={t("linkedinImport.title")}
            status={
              held === undefined
                ? undefined
                : held === 0
                  ? {
                      tone: statusTone("neutral"),
                      label: t("linkedinImport.notImported"),
                    }
                  : {
                      tone: statusTone("done"),
                      label: plural("linkedinImport.importedCount", held, {
                        count: formatNumber(held, locale),
                      }),
                    }
            }
            // What happens to the file, said before the member drops it: their
            // own address book is going into a company system.
            facts={t("linkedinImport.sub")}
          >
            <ConnectionsImport />
          </ProviderRow>
        </ProviderList>
      </PanelBody>
      <SettingList bleed="settings">
        <LinkedInProfileRow />
        <ReachVisibilityRow />
      </SettingList>
      <PanelBody>
        {/* LinkedIn's archive holds a dozen CSVs and the wrong one fails with
            a parse error, so the way to the right file stays one click away. */}
        <Disclosure summary={t("linkedinImport.importLabel")}>
          <p>{t("linkedinImport.whichFile")}</p>
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}

/** The file, picked or dropped, and what the import made of it, under the row. */
function ConnectionsImport() {
  const t = useT();
  const importer = useImportConnections();
  const [file, setFile] = useState<File | undefined>(undefined);
  return (
    <Stack>
      <FileDropzone
        label={t("linkedinImport.choose")}
        emptyLabel={t("linkedinImport.dropHint")}
        accept=".csv,text/csv"
        file={file}
        onPick={(picked) => {
          setFile(picked);
          importer.mutate(picked);
        }}
      />
      {importer.isIdle ? null : <ImportOutcome importer={importer} />}
    </Stack>
  );
}

/** What the import DID: a report under the row, not a setting beside it. */
function ImportOutcome({
  importer,
}: Readonly<{ importer: ReturnType<typeof useImportConnections> }>) {
  const t = useT();
  if (importer.isPending) {
    return <PendingBody visible label={t("linkedinImport.working")} />;
  }
  if (importer.isError) {
    return (
      <Callout
        kind="outcome"
        tone="danger"
        title={t("linkedinImport.importFailed")}
      >
        {problemMessageOf(importer.error, t)}
      </Callout>
    );
  }
  return importer.data === undefined ? null : (
    <ImportResult summary={importer.data} />
  );
}

// ImportResult states what happened in the terms someone checking the import
// would ask. Skipped rows are shown rather than hidden: a file half-ignored
// under a success message is worse than a refusal.
function ImportResult({ summary }: Readonly<{ summary: ImportSummary }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <StatStrip testId="linkedin-import-result" density="compact">
        <StatCard
          label={t("linkedinImport.imported")}
          value={formatNumber(summary.imported, locale)}
        />
        <StatCard
          label={t("linkedinImport.confirmed")}
          value={formatNumber(summary.confirmed, locale)}
        />
        <StatCard
          label={t("linkedinImport.suggested")}
          value={formatNumber(summary.suggested, locale)}
        />
        {summary.skipped > 0 && (
          <StatCard
            label={t("linkedinImport.skipped")}
            value={formatNumber(summary.skipped, locale)}
          />
        )}
      </StatStrip>
      {/* Zero matches on a new workspace is expected rather than wrong: the
          contacts an export matches arrive with mail capture over the hours
          after it. Saying so stops it reading as a broken import. */}
      {summary.confirmed + summary.suggested === 0 && summary.imported > 0 && (
        <p className="t-caption">{t("linkedinImport.noMatchesYet")}</p>
      )}
    </>
  );
}
