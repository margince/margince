// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { Download } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Button, SegmentedControl } from "../design-system/atoms";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { Eyebrow } from "../design-system/eyebrow";
import { PanelPlate } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import { throwProblem, WriteRefused } from "./common";
import { downloadBytes, filenameFromDisposition } from "./download";

const SKILL_FILENAME = "margince-skill.zip";

// The passport itself never appears here, even right after a mint: a pasted
// example lands in shell history, and the variable keeps it out.
const PASSPORT_VARIABLE = "MARGINCE_PASSPORT";

const SNIPPET_LANGUAGES = ["curl", "python", "javascript"] as const;
type SnippetLanguage = (typeof SNIPPET_LANGUAGES)[number];

// Product names, typed as each tool spells them, so not translated.
const SNIPPET_LANGUAGE_NAMES: Record<SnippetLanguage, string> = {
  curl: "curl",
  python: "Python",
  javascript: "JavaScript",
};

export function passportSnippet(
  language: SnippetLanguage,
  apiBaseUrl: string,
): string {
  const url = `${apiBaseUrl.replace(/\/+$/, "")}/companies?limit=5`;
  switch (language) {
    case "curl":
      return [
        `curl "${url}" \\`,
        `  -H "Authorization: Bearer $${PASSPORT_VARIABLE}"`,
      ].join("\n");
    case "python":
      return [
        "import os",
        "import requests",
        "",
        "response = requests.get(",
        `    "${url}",`,
        `    headers={"Authorization": "Bearer " + os.environ["${PASSPORT_VARIABLE}"]},`,
        ")",
        "print(response.json())",
      ].join("\n");
    case "javascript":
      return [
        `const response = await fetch("${url}", {`,
        `  headers: { Authorization: \`Bearer \${process.env.${PASSPORT_VARIABLE}}\` },`,
        "});",
        "console.log(await response.json());",
      ].join("\n");
  }
}

export function SkillDownloadButton() {
  const t = useT();
  const download = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.GET("/agent-bundle", {
        parseAs: "blob",
      });
      if (error) throwProblem(error);
      return {
        bytes: data,
        filename: filenameFromDisposition(
          response.headers.get("content-disposition"),
          SKILL_FILENAME,
        ),
      };
    },
    onSuccess: ({ bytes, filename }) =>
      downloadBytes(bytes, filename, "application/zip"),
  });
  return (
    <>
      <Button pending={download.isPending} onClick={() => download.mutate()}>
        <Download aria-hidden />
        {t("settings.skillDownload")}
      </Button>
      <WriteRefused
        titleKey="settings.skillDownloadFailed"
        error={download.error}
      />
    </>
  );
}

// `holdsClipboard` is false once a sibling copy control on the same surface has
// written since, so two buttons never both read Copied over one clipboard.
export function PassportSnippet({
  apiBaseUrl,
  onCopied,
  holdsClipboard = true,
}: Readonly<{
  apiBaseUrl: string;
  onCopied?: () => void;
  holdsClipboard?: boolean;
}>) {
  const t = useT();
  const [language, setLanguage] = useState<SnippetLanguage>("curl");
  const code = passportSnippet(language, apiBaseUrl);
  const labels = {
    copy: t("settings.snippetCopy"),
    copied: t("settings.snippetCopied"),
    remedy: t("settings.snippetCopyFailed"),
  };
  const copy = useClipboardCopy(code, labels, onCopied);
  const name = SNIPPET_LANGUAGE_NAMES[language];
  return (
    <div className="passport-stack">
      <div className="passport-snippet-bar">
        <SegmentedControl
          options={SNIPPET_LANGUAGES}
          value={language}
          onChange={setLanguage}
          labels={SNIPPET_LANGUAGE_NAMES}
          label={t("settings.snippetLanguage")}
        />
        <Button onClick={copy.copy}>
          {copy.copied && holdsClipboard ? labels.copied : labels.copy}
        </Button>
      </div>
      <CopyableText
        text={code}
        label={t("settings.snippetLabel", { language: name })}
        testId="passport-snippet"
      />
      {copy.notice}
      <p className="t-caption">
        {t("settings.snippetFoot", { variable: PASSPORT_VARIABLE })}
      </p>
    </div>
  );
}

// The two ways to put a passport to work, shared by the card and the mint
// dialog. The code row waits for the API address the server states.
export function PassportUses({
  apiBaseUrl,
  onSnippetCopied,
  snippetHoldsClipboard,
}: Readonly<{
  apiBaseUrl: string | undefined;
  onSnippetCopied?: () => void;
  snippetHoldsClipboard?: boolean;
}>) {
  const t = useT();
  return (
    <SettingList>
      <SettingRow
        label={t("settings.passportUseAi")}
        description={t("settings.passportUseAiDetail")}
        control={<SkillDownloadButton />}
      />
      {apiBaseUrl !== undefined && (
        <SettingRow
          layout="stack"
          label={t("settings.passportUseCode")}
          description={t("settings.passportUseCodeDetail")}
          control={
            <PassportSnippet
              apiBaseUrl={apiBaseUrl}
              onCopied={onSnippetCopied}
              holdsClipboard={snippetHoldsClipboard}
            />
          }
        />
      )}
    </SettingList>
  );
}

// The one sight of a new passport, with the ways to use it under it. The
// dialog's live region says only that it was created; the value stays out of it.
export function MintedPassport({
  token,
  apiBaseUrl,
}: Readonly<{ token: string; apiBaseUrl: string | undefined }>) {
  const t = useT();
  const [holder, setHolder] = useState<"token" | "snippet" | null>(null);
  const labels = {
    copy: t("settings.tokenCopy"),
    copied: t("settings.tokenCopied"),
    remedy: t("settings.tokenCopyFailed"),
  };
  const copy = useClipboardCopy(token, labels, () => setHolder("token"));
  return (
    <>
      <PanelPlate className="passport-stack">
        <CopyableText
          text={token}
          label={t("settings.token")}
          testId="passport-token"
        />
        <div>
          <Button onClick={copy.copy}>
            {copy.copied && holder === "token" ? labels.copied : labels.copy}
          </Button>
        </div>
        {copy.notice}
      </PanelPlate>
      <Eyebrow as="h3">{t("settings.passportNext")}</Eyebrow>
      <PassportUses
        apiBaseUrl={apiBaseUrl}
        onSnippetCopied={() => setHolder("snippet")}
        snippetHoldsClipboard={holder === "snippet"}
      />
    </>
  );
}
