// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ExternalLink } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import { AppLink } from "../design-system/applink";
import { Button } from "../design-system/button";
import { CopyableText, useClipboardCopy } from "../design-system/clipboardcopy";
import { FactList } from "../design-system/factlist";
import { Heading } from "../design-system/heading";
import { Logomark } from "../design-system/logomark";
import { Modal } from "../design-system/modal";
import { OffsiteLink } from "../design-system/offsitelink";
import { type Translator, useT } from "../i18n";
import {
  type BuildRelease,
  RELEASE_NOTES_HREF,
  REPOSITORY,
} from "../lib/buildrelease";
import { comparableRelease, shortRevision } from "../lib/release";
import "./aboutmargince.css";

const WEBSITE_HREF = "https://margince.com/";
const LICENSING_HREF = `${REPOSITORY}/blob/main/LICENSING.md`;
const THIRD_PARTY_LICENSES_HREF = "/licenses/THIRD-PARTY-NOTICES.txt";

function knownRevision(build: BuildRelease): string | undefined {
  return comparableRelease(build.revision) ? build.revision : undefined;
}

/**
 * The build as one line of plain text, for a problem report. English in every
 * locale: it is pasted into the core's issue tracker.
 */
export function buildReport(build: BuildRelease): string {
  const revision = knownRevision(build);
  const commit = revision ? ` (commit ${revision})` : "";
  switch (build.kind) {
    case "instance":
      return build.instance
        ? `Margince core ${build.core}, instance ${build.instance}${commit}`
        : `Margince core ${build.core}${commit}`;
    case "core":
      return `Margince ${build.release}${commit}`;
    default:
      return `Margince development build${commit}`;
  }
}

function NewTabLink({
  href,
  children,
}: Readonly<{ href: string; children: ReactNode }>) {
  const t = useT();
  const content = (
    <>
      {children}
      <span className="sr-only"> {t("common.opensNewTab")}</span>
      <ExternalLink aria-hidden />
    </>
  );
  if (href.startsWith("/")) {
    return (
      <AppLink
        className="link-button"
        href={href}
        target="_blank"
        rel="noopener noreferrer"
      >
        {content}
      </AppLink>
    );
  }
  return <OffsiteLink href={href}>{content}</OffsiteLink>;
}

/** The release with its short commit, as the one line a reader quotes. */
function versionLine(build: BuildRelease, t: Translator): string {
  const short = shortRevision(knownRevision(build));
  let release: string;
  if (build.kind === "core") {
    release = build.release;
  } else if (build.kind === "instance") {
    release = build.instance ?? t("about.coreRelease", { version: build.core });
  } else {
    release = t("about.devBuild");
  }
  return short ? `${release} · ${short}` : release;
}

function versionNote(build: BuildRelease, t: Translator): ReactNode {
  return build.kind === "instance" && build.instance
    ? t("about.coreRelease", { version: build.core })
    : undefined;
}

const COPIED_MS = 3000;

/** The version, pressed to copy the whole build line with its full commit. */
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
  const revision = build.kind === "core" ? knownRevision(build) : undefined;
  return (
    <div className="aboutmargince-version">
      <Button variant="link" align="start" onClick={copy.copy}>
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

/**
 * What this build of Margince is, under which license, and where it lives.
 * `surface` sets the space around it for the dialog or the settings page.
 */
export function AboutMargince({
  build,
  surface,
}: Readonly<{ build: BuildRelease; surface: "dialog" | "page" }>) {
  const t = useT();
  return (
    <div className={`aboutmargince aboutmargince-in-${surface}`}>
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
        <AboutMargince build={build} surface="dialog" />
      </div>
    </Modal>
  );
}
