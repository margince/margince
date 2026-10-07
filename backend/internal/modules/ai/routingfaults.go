// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// routingFault is one refused key in a routing document, addressed by its path
// from the document root (`tiers.cheap_cloud.routing.provider.sort.by`), so the
// editor can point at the line that holds it.
type routingFault struct {
	Path    string
	Code    string
	Message string
}

// routingFaults holds the refused keys of one write. It is the plural field
// fault, so the 422 lists each path instead of collapsing them into one
// `ai.routing` reason the editor cannot place.
type routingFaults []routingFault

func (f routingFaults) Error() string {
	parts := make([]string, 0, len(f))
	for _, fault := range f {
		parts = append(parts, strings.TrimPrefix(fault.Path+" "+fault.Message, " "))
	}
	return "ai: routing config: " + strings.Join(parts, "; ")
}

// FieldFaults reports each path as its own field. A fault with no path is the
// document as a whole, which the caller changes by its setting key.
func (f routingFaults) FieldFaults() []apperrors.FieldRefusal {
	out := make([]apperrors.FieldRefusal, 0, len(f))
	for _, fault := range f {
		field := fault.Path
		if field == "" {
			field = RoutingKey
		}
		out = append(out, apperrors.FieldRefusal{Field: field, Code: fault.Code, Message: fault.Message})
	}
	return out
}

func faultAt(path, code, msg string) routingFaults {
	return routingFaults{{Path: path, Code: code, Message: msg}}
}

func invalidAt(path, msg string) routingFaults {
	return faultAt(path, settings.CodeInvalidValue, msg)
}

// joinFaults flattens refusals into one list, nil when there are none.
func joinFaults(errs ...error) error {
	if out := faultsOf(errs...); len(out) > 0 {
		return out
	}
	return nil
}

// faultsOf flattens refusals. A nil is skipped, and an error that names no
// path becomes a fault on the whole document rather than being dropped, so a
// refusal never vanishes by being joined.
func faultsOf(errs ...error) routingFaults {
	var out routingFaults
	for _, err := range errs {
		if err == nil {
			continue
		}
		var faults routingFaults
		if errors.As(err, &faults) {
			out = append(out, faults...)
			continue
		}
		out = append(out, routingFault{Code: settings.CodeInvalidValue, Message: err.Error()})
	}
	return out
}

// JoinRoutingFaults is joinFaults for the transport, which reads each lane's
// routing value before the store sees the document.
func JoinRoutingFaults(errs ...error) error { return joinFaults(errs...) }

// joinPath appends one key to a path, leaving a root path unprefixed.
func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
