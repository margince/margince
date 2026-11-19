// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apac

// The messaging rules are compiled from policy/ into messagingrules_gen.go.
// This unit is outside the root go.work, so go generate runs with GOWORK=off,
// and the generator needs the workspace back to find its own module.
//go:generate env GOWORK= go run -C ../../backend/tools ./gen-messaging-rules -dir ../../extensions/apac
