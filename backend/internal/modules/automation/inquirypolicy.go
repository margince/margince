// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const inquiryRecipeName = provenance.WebsiteInquirySource

// ParseInquiryPolicy keeps every policy reader on the same typed configuration.
func ParseInquiryPolicy(raw json.RawMessage) (crmcontracts.WebsiteInquiryPolicy, error) {
	var policy crmcontracts.WebsiteInquiryPolicy
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return policy, inquiryParamError("", "must match the website inquiry policy")
	}
	if routing, present := fields["routing"]; !present || bytes.Equal(bytes.TrimSpace(routing), []byte("null")) {
		return policy, inquiryParamError("routing", "must declare how inquiries are routed")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return policy, &ParamError{Field: "params", Reason: "must match the website inquiry policy"}
	}
	return policy, checkInquiryPolicy(policy)
}

func checkInquiryPolicy(p crmcontracts.WebsiteInquiryPolicy) error {
	switch {
	case ids.UUID(p.FormId) == ids.Nil:
		return inquiryParamError("form_id", "must name a website form")
	case ids.UUID(p.CoordinatorId) == ids.Nil:
		return inquiryParamError("coordinator_id", "must name a colleague for review and fallback")
	case !p.Mode.Valid():
		return inquiryParamError("mode", "must be one_colleague, rotate or coordinator")
	case strings.TrimSpace(p.TaskTitle) == "" || utf8.RuneCountInString(p.TaskTitle) > 200:
		return inquiryParamError("task_title", "must contain between 1 and 200 characters")
	case p.DueInDays < 1 || p.DueInDays > 30:
		return inquiryParamError(paramDueInDays, "must be between 1 and 30 calendar days")
	}
	if err := values.ZoneName(p.Timezone); err != nil {
		return inquiryParamError(paramTaskTimezone, "must be a valid IANA timezone name")
	}
	if p.RequestTypeField != nil && (strings.TrimSpace(*p.RequestTypeField) == "" || utf8.RuneCountInString(*p.RequestTypeField) > 100) {
		return inquiryParamError("request_type_field", "must name a mapped picklist field")
	}
	return checkInquiryRouting(p)
}

func checkInquiryRouting(p crmcontracts.WebsiteInquiryPolicy) error {
	if p.Routing.CapPerOwner != nil || (p.Routing.Rules != nil && len(*p.Routing.Rules) != 0) {
		return inquiryParamError("routing", "website inquiries support selected colleagues without capacity or territory rules")
	}
	owners := map[ids.UUID]bool{}
	if p.Routing.Owners != nil {
		for _, owner := range *p.Routing.Owners {
			id := ids.UUID(owner)
			if id == ids.Nil || owners[id] {
				return inquiryParamError("routing.owners", "must name distinct colleagues")
			}
			owners[id] = true
		}
	}
	switch p.Mode {
	case crmcontracts.WebsiteInquiryModeOneColleague:
		if len(owners) != 1 {
			return inquiryParamError("routing.owners", "must name exactly one colleague")
		}
	case crmcontracts.WebsiteInquiryModeRotate:
		if len(owners) == 0 {
			return inquiryParamError("routing.owners", "must name at least one colleague")
		}
	case crmcontracts.WebsiteInquiryModeCoordinator:
		if len(owners) != 0 || p.Routing.CapPerOwner != nil || (p.Routing.Rules != nil && len(*p.Routing.Rules) != 0) {
			return inquiryParamError("routing", "coordinator review leaves the lead unowned")
		}
	}

	raw, err := json.Marshal(p.Routing)
	if err != nil {
		return err
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return err
	}
	err = validateLeadRoutingParams(params)
	if field, ok := errors.AsType[*ParamError](err); ok {
		return &ParamError{Field: strings.Replace(field.Field, "params.", "params.routing.", 1), Reason: field.Reason}
	}
	return err
}

func inquiryParamError(field, reason string) error {
	if field == "" {
		return &ParamError{Field: "params", Reason: reason}
	}
	return &ParamError{Field: "params." + field, Reason: reason}
}
