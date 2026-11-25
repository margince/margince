// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// statedWhyLimit is the schema's maxLength for why, in characters.
const statedWhyLimit = 600

// whyProperty is the argument a field-changing tool takes for the reason
// behind the change. The surface reads it from every tool that declares it.
// A call that runs keeps it on its audit rows. A staged call's card leads with it.
const whyProperty = `"why":{"type":"string","minLength":1,"maxLength":600,"description":"Why you are making this change, ` +
	`in one sentence the user reads beside it. Name who, which record or message, and the fact that prompted it, ` +
	`for example: Anna Weber wrote on 14 Nov that the rollout moves to March."}`

// checkStatedWhy refuses a reason the schema would. A blank one reads as no
// reason at all. A long one would be cut where the reader sees it.
func checkStatedWhy(why string) error {
	switch {
	case strings.TrimSpace(why) == "":
		return &BadArgsError{Cause: errors.New("`why` is blank; say in one sentence why you are making this change"), Field: "why"}
	case utf8.RuneCountInString(why) > statedWhyLimit:
		return &BadArgsError{Cause: errors.New("`why` is longer than 600 characters; say it in one sentence"), Field: "why"}
	}
	return nil
}

// statedWhy is the call's own reason, trimmed, or "" when it gave none.
func statedWhy(args json.RawMessage) string {
	var call struct {
		Why string `json:"why"`
	}
	if json.Unmarshal(args, &call) != nil {
		return ""
	}
	return strings.TrimSpace(call.Why)
}
