// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/// <reference types="vite-plugin-svgr/client" />

import type { ComponentProps, ComponentType, FunctionComponent } from "react";
// Copied unchanged from @material-symbols/svg-500 0.48.1, rounded (Apache-2.0,
// LICENSE beside them). A new mark is one SVG copied in, its sha256 pinned in
// materialsymbols.test.tsx, and one import here.
import CheckCircleSvg from "./material-symbols/check_circle-fill.svg?react";
import ErrorSvg from "./material-symbols/error-fill.svg?react";
import InfoSvg from "./material-symbols/info-fill.svg?react";
import Lightbulb2Svg from "./material-symbols/lightbulb_2-fill.svg?react";
import WarningSvg from "./material-symbols/warning-fill.svg?react";

/** A glyph from either icon source, so a slot that takes one takes the other. */
export type Glyph = ComponentType<Readonly<{ size?: number }>>;

type MaterialSymbolProps = ComponentProps<"svg"> &
  Readonly<{ size?: number | string }>;

function materialSymbol(
  Svg: FunctionComponent<ComponentProps<"svg">>,
  name: string,
): FunctionComponent<MaterialSymbolProps> {
  function MaterialSymbol({ size = 24, ...props }: MaterialSymbolProps) {
    return (
      <Svg
        width={size}
        height={size}
        fill="currentColor"
        aria-hidden="true"
        data-symbol={name}
        {...props}
      />
    );
  }
  MaterialSymbol.displayName = `MaterialSymbol(${name})`;
  return MaterialSymbol;
}

export const InfoFilled = materialSymbol(InfoSvg, "info");
export const WarningFilled = materialSymbol(WarningSvg, "warning");
export const ErrorFilled = materialSymbol(ErrorSvg, "error");
export const CheckCircleFilled = materialSymbol(CheckCircleSvg, "check-circle");
export const Lightbulb2Filled = materialSymbol(Lightbulb2Svg, "lightbulb-2");
