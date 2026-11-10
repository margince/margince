// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import "strings"

// The sale categories a won deal's value splits into. Software is a product
// sale and implementation a service. Every other kind is unresolved, and so
// are products nobody classified and the part no line carries.
const (
	Product    = "product"
	Service    = "service"
	Unresolved = "unresolved"
)

// InvalidKind groups a deal whose lines add up to more than its value. Its
// whole value is shown under it, never spread over the lines.
const InvalidKind = "invalid"

// Category places a commercial-mix group key in its sale category.
func Category(groupKey string) string {
	switch strings.TrimPrefix(groupKey, "kind:") {
	case "software":
		return Product
	case "implementation":
		return Service
	}
	return Unresolved
}

// The reporting contexts a reading takes its facts from: the period's window,
// and the open pipeline as it stands now.
const (
	contextInterval = "interval"
	contextState    = "state"
)
