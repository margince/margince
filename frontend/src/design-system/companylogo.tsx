// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import "./companylogo.css";

/**
 * A company's full logo in a slot whose caller sizes. Unlike Avatar this keeps
 * a wordmark wide; both share the same rule that the fallback stays underneath
 * until the image has painted and returns if the image fails.
 */
export function CompanyLogo({
  name,
  src,
  fallback,
}: Readonly<{
  name: string;
  src?: string | null;
  fallback: ReactNode;
}>) {
  const [brokenSrc, setBrokenSrc] = useState<string | null>(null);
  const [paintedSrc, setPaintedSrc] = useState<string | null>(null);
  const broken = Boolean(src) && brokenSrc === src;
  const painted = Boolean(src) && paintedSrc === src && !broken;

  return (
    <span className="company-logo" role="img" aria-label={name}>
      {src && !broken ? (
        <img
          className="company-logo-img"
          src={src}
          alt=""
          aria-hidden="true"
          onLoad={() => setPaintedSrc(src)}
          onError={() => setBrokenSrc(src)}
        />
      ) : null}
      {!painted && (
        <span className="company-logo-fallback" aria-hidden="true">
          {fallback}
        </span>
      )}
    </span>
  );
}
