// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func inquiryCatalogEntry() CatalogEntry {
	return CatalogEntry{
		Group: crmcontracts.AutomationCatalogEntryGroupInquiries,
		Key:   inquiryRecipeName, Name: "Handle website inquiries",
		Description: "Route accepted website inquiries and give a colleague one task to answer each request.",
		Trigger:     eventActivityCaptured, Action: string(ActionTypeCreateTask), Tier: tierAutoExecute,
		ParamsSchema: inquiryPolicySchema(), Validate: validateInquiryParams,
	}
}

func validateInquiryParams(params map[string]any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	_, err = ParseInquiryPolicy(raw)
	return err
}

func inquiryPolicySchema() map[string]any {
	uuid := func() map[string]any { return map[string]any{schemaKeyType: schemaTypeString, schemaKeyFormat: "uuid"} }
	return map[string]any{
		schemaKeyType: schemaTypeObject, schemaKeyAdditionalProps: false,
		schemaKeyRequired: []string{"form_id", "mode", "routing", "coordinator_id", "task_title", paramDueInDays, paramTaskTimezone},
		schemaKeyProperties: map[string]any{
			"form_id": uuid(), "coordinator_id": uuid(),
			"mode":               map[string]any{schemaKeyType: schemaTypeString, schemaKeyEnum: []crmcontracts.WebsiteInquiryMode{crmcontracts.WebsiteInquiryModeOneColleague, crmcontracts.WebsiteInquiryModeRotate, crmcontracts.WebsiteInquiryModeCoordinator}},
			"routing":            leadRoutingSchema(),
			"task_title":         map[string]any{schemaKeyType: schemaTypeString, "minLength": 1, "maxLength": 200},
			paramDueInDays:       intParamProperty(1, 30, "Calendar days after receipt before the task is due."),
			paramTaskTimezone:    taskTimezoneSchema(),
			"request_type_field": map[string]any{schemaKeyType: schemaTypeString, "minLength": 1, "maxLength": 100},
		},
	}
}
