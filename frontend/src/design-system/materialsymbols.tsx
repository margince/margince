// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/// <reference types="vite-plugin-svgr/client" />

import CheckCircleSvg from "@material-symbols/svg-500/rounded/check_circle-fill.svg?react";
import ErrorSvg from "@material-symbols/svg-500/rounded/error-fill.svg?react";
import InfoSvg from "@material-symbols/svg-500/rounded/info-fill.svg?react";
import Lightbulb2Svg from "@material-symbols/svg-500/rounded/lightbulb_2-fill.svg?react";
import WarningSvg from "@material-symbols/svg-500/rounded/warning-fill.svg?react";
import type { ComponentProps, ComponentType, FunctionComponent } from "react";

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
