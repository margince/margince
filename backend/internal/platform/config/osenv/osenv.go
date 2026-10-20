// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package osenv holds the product's one os.Getenv apart from config. A test
// lane's root reads the environment here, because config's stored settings
// import the RBAC gate, which imports packages those lanes test.
package osenv

import "os"

// Get reads one variable, answering "" when it is not set.
func Get(name string) string { return os.Getenv(name) }
