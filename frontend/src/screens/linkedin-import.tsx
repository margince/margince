// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Button,
  Disclosure,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { ProviderMark } from "../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../design-system/providerrow";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { LinkedInReachSection } from "./linkedin-reach";
import {
  LINKEDIN_ACCOUNT_KEY,
  useSaveLinkedInAccount,
} from "./onboarding-conversation/use-linkedin-account";
import "./linkedin-import.css";

// The LinkedIn network (ADR-0078 §2.1b): the member's own Connections.csv
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
      // Sent as multipart by hand rather than through the typed client: the
      // generated client serializes JSON bodies, and this endpoint takes a
      // file part.
      const body = new FormData();
      body.append("file", file);
      // contract-fetch:allow multipart: see the note above
      const response = await fetch("/v1/me/linkedin-connections", {
        method: "POST",
        body,
        credentials: "include",
      });
      const payload = await response.json().catch(() => undefined);
      if (!response.ok) {
        throwProblem(payload);
      }
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
      const { data, error } = await api.GET("/me/linkedin-account", {});
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// Your own LinkedIn profile, shown back so it can be corrected; the API has no
// path to another member's. Value and verb on the row, the field in a modal, so
// the row says what it is set to rather than only that something is editable.
function LinkedInProfileRow() {
  const t = useT();
  const account = useLinkedInAccount();
  const save = useSaveLinkedInAccount();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<string | null>(null);
  const headingId = useId();

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
      <Modal open={editing} onClose={close} labelledBy={headingId}>
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("linkedinImport.editProfileTitle")}
        </Heading>
        <form
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
          {/* Field + TextInput, not a hand-rolled label wrapping a bare <input>:
              this one box had its own label type, its own padding, its own border
              and its own focus ring, none of which agreed with the field beside it
              in any other dialog on the tab. */}
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
          <div className="actions">
            <Button type="button" onClick={close} disabled={save.isPending}>
              {t("create.cancel")}
            </Button>
            {/* An unchanged URL and a save in flight are two different
                unavailabilities, and the design system draws them differently:
                `disabled` for the precondition the reader can fix by typing,
                `pending` for the write they have already started, which keeps
                the button focusable so the wait is announced from it. */}
            <Button
              variant="primary"
              type="submit"
              disabled={!dirty}
              pending={save.isPending}
            >
              {t("linkedinImport.saveProfile")}
            </Button>
          </div>
        </form>
      </Modal>
    </>
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
  const importer = useImportConnections();
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
                  ? { tone: "default", label: t("linkedinImport.notImported") }
                  : {
                      tone: "success",
                      label: plural("linkedinImport.importedCount", held, {
                        count: formatNumber(held, locale),
                      }),
                    }
            }
            // What happens to the file, said before the member presses: their
            // own address book is going into a company system.
            facts={<span className="t-sub">{t("linkedinImport.sub")}</span>}
            action={<ConnectionsFilePicker onFile={importer.mutate} />}
          >
            {importer.isIdle ? undefined : (
              <ImportOutcome importer={importer} />
            )}
          </ProviderRow>
        </ProviderList>
        <SettingList>
          <LinkedInProfileRow />
        </SettingList>
        <LinkedInReachSection />
        {/* LinkedIn's archive holds a dozen CSVs and the wrong one fails with
            a parse error, so the way to the right file stays one click away. */}
        <Disclosure summary={t("linkedinImport.importLabel")}>
          <p>{t("linkedinImport.whichFile")}</p>
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}

/** The file picker in the row's action slot: a real <label> drives a hidden
 * input, because the input's own button cannot be styled or translated. */
function ConnectionsFilePicker({
  onFile,
}: Readonly<{ onFile: (file: File) => void }>) {
  const t = useT();
  const inputId = useId();
  const [fileName, setFileName] = useState<string | null>(null);
  return (
    <div className="li-import-picker">
      {fileName && <span className="t-sub">{fileName}</span>}
      <label className="li-import-button" htmlFor={inputId}>
        {t("linkedinImport.choose")}
      </label>
      <input
        id={inputId}
        type="file"
        accept=".csv,text/csv"
        data-testid="linkedin-import-file"
        onChange={(e) => {
          const file = e.target.files?.[0];
          setFileName(file?.name ?? null);
          if (file) {
            onFile(file);
          }
        }}
      />
    </div>
  );
}

/** What the import DID: a report under the row, not a setting beside it. */
function ImportOutcome({
  importer,
}: Readonly<{ importer: ReturnType<typeof useImportConnections> }>) {
  const t = useT();
  if (importer.isPending) {
    return <p className="t-sub">{t("linkedinImport.working")}</p>;
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
      <dl data-testid="linkedin-import-result" className="li-import-result">
        <div>
          <dt>{t("linkedinImport.imported")}</dt>
          <dd>{formatNumber(summary.imported, locale)}</dd>
        </div>
        <div>
          <dt>{t("linkedinImport.confirmed")}</dt>
          <dd>{formatNumber(summary.confirmed, locale)}</dd>
        </div>
        <div>
          <dt>{t("linkedinImport.suggested")}</dt>
          <dd>{formatNumber(summary.suggested, locale)}</dd>
        </div>
        {summary.skipped > 0 && (
          <div>
            <dt>{t("linkedinImport.skipped")}</dt>
            <dd>{formatNumber(summary.skipped, locale)}</dd>
          </div>
        )}
      </dl>
      {/* Zero matches on a new workspace is expected rather than wrong: the
          contacts an export matches arrive with mail capture over the hours
          after it. Saying so stops it reading as a broken import. */}
      {summary.confirmed + summary.suggested === 0 && summary.imported > 0 && (
        <p className="t-sub">{t("linkedinImport.noMatchesYet")}</p>
      )}
    </>
  );
}
