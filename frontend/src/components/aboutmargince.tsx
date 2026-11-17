// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useId, useState } from "react";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/button";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Logomark } from "../design-system/logomark";
import { Modal } from "../design-system/modal";
import { NewTabMark, OffsiteLink } from "../design-system/offsitelink";
import { type Translator, useT } from "../i18n";
import {
  type BuildRelease,
  buildReport,
  RELEASE_NOTES_HREF,
  REPOSITORY,
} from "../lib/buildrelease";
import { shortRevision } from "../lib/release";
import "./aboutmargince.css";

const WEBSITE_HREF = "https://margince.com/";
const LICENSING_HREF = `${REPOSITORY}/blob/main/LICENSING.md`;
const THIRD_PARTY_LICENSES_HREF = "/licenses/THIRD-PARTY-NOTICES.txt";

function NewTabLink({
  href,
  children,
}: Readonly<{ href: string; children: ReactNode }>) {
  if (href.startsWith("/")) {
    return (
      <AppLink
        className="link-button"
        href={href}
        target="_blank"
        rel="noopener noreferrer"
      >
        {children}
        <NewTabMark />
      </AppLink>
    );
  }
  return (
    <OffsiteLink href={href}>
      {children}
      <NewTabMark />
    </OffsiteLink>
  );
}

function versionLine(build: BuildRelease, t: Translator): string {
  let release: string;
  if (build.kind === "core") {
    release = build.release;
  } else if (build.kind === "instance") {
    release = build.instance ?? t("about.instanceBuild");
  } else {
    release = t("about.devBuild");
  }
  const short = shortRevision(build.revision);
  return short ? `${release} · ${short}` : release;
}

function versionNote(build: BuildRelease, t: Translator): ReactNode {
  return build.kind === "instance"
    ? t("about.coreRelease", { version: build.core })
    : undefined;
}

const COPIED_MS = 3000;

function VersionCopy({ build }: Readonly<{ build: BuildRelease }>) {
  const t = useT();
  const report = buildReport(build);
  const [copies, setCopies] = useState<number | null>(null);
  const copy = useClipboardCopy(
    report,
    {
      copy: t("about.copyHint"),
      copied: t("about.copiedVersion"),
      remedy: t("about.copyRemedy"),
    },
    () => setCopies((count) => (count ?? 0) + 1),
  );
  useEffect(() => {
    if (copies === null) {
      return;
    }
    const timer = setTimeout(() => setCopies(null), COPIED_MS);
    return () => clearTimeout(timer);
  }, [copies]);
  const copied = copies !== null;
  // Only a core build's commit is in the core repository; an instance's is in
  // the repository of whoever built it.
  const revision = build.kind === "core" ? build.revision : undefined;
  return (
    <div className="aboutmargince-version">
      <Button
        variant="link"
        align="start"
        onClick={() => {
          setCopies(null);
          copy.copy();
        }}
      >
        {copied ? (
          t("about.copiedVersion")
        ) : (
          <>
            {versionLine(build, t)}
            <span className="sr-only"> {t("about.copyHint")}</span>
          </>
        )}
      </Button>
      <span className="sr-only" role="status">
        {copied ? t("about.copiedVersion") : ""}
      </span>
      {revision && (
        <NewTabLink href={`${REPOSITORY}/commit/${revision}`}>
          {t("about.viewCommit")}
        </NewTabLink>
      )}
      {copy.notice}
      {copy.notice && <CopyableText text={report} label={t("about.version")} />}
    </div>
  );
}

/** What this build of Margince is, under which license, and where it lives. */
export function AboutMargince({ build }: Readonly<{ build: BuildRelease }>) {
  const t = useT();
  return (
    <div className="aboutmargince">
      <div className="aboutmargince-brand">
        <Logomark />
        {t("shell.logoAria")}
      </div>
      <FactList
        flat
        className="aboutmargince-facts"
        facts={[
          {
            key: "version",
            term: t("about.version"),
            value: <VersionCopy build={build} />,
            note: versionNote(build, t),
          },
          {
            key: "license",
            term: t("about.license"),
            value: (
              <NewTabLink href={LICENSING_HREF}>
                {t("about.licenseName")}
              </NewTabLink>
            ),
            note: t("about.licenseConversion"),
          },
          {
            key: "copyright",
            term: t("about.copyrightTerm"),
            value: t("about.copyright"),
          },
          {
            key: "links",
            term: t("about.links"),
            value: (
              <ul className="aboutmargince-links">
                <li>
                  <NewTabLink href={WEBSITE_HREF}>
                    {t("about.website")}
                  </NewTabLink>
                </li>
                <li>
                  <NewTabLink href={REPOSITORY}>{t("about.source")}</NewTabLink>
                </li>
                <li>
                  <NewTabLink href={RELEASE_NOTES_HREF}>
                    {t("shell.help.releaseNotes")}
                  </NewTabLink>
                </li>
                <li>
                  <NewTabLink href={THIRD_PARTY_LICENSES_HREF}>
                    {t("about.thirdParty")}
                  </NewTabLink>
                </li>
              </ul>
            ),
          },
        ]}
      />
    </div>
  );
}

export function AboutMarginceDialog({
  build,
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  build: BuildRelease;
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
      returnFocusTo={returnFocusTo}
    >
      <div className="form-stack">
        <Heading size="large" id={titleId}>
          {t("about.title")}
        </Heading>
        <div className="aboutmargince-dialogbody">
          <AboutMargince build={build} />
        </div>
      </div>
    </Modal>
  );
}
