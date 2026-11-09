// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// React passes the Element Timing attribute to the DOM as written, but its DOM
// types do not name it.
declare module "react" {
  interface HTMLAttributes<T> {
    elementtiming?: string;
  }
}

/**
 * The Element Timing mark on Home's day content. The Home benchmark
 * (`make bench-home`) times when that content paints by this name.
 */
export const HOME_DAY_TIMING = "home-day";
