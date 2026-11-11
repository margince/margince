// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import "encoding/json"

func quietReminderSchema(daysKey string, defaultDays int, populationKey, format string) map[string]any {
	population := map[string]any{schemaKeyType: schemaTypeString, schemaKeyFormat: format}
	if format == "record_list" {
		population["x-entity-types"] = QuietCustomerTypes()
	}
	return map[string]any{schemaKeyType: schemaTypeObject, schemaKeyAdditionalProps: false,
		schemaKeyProperties: map[string]any{daysKey: intParamProperty(defaultDays, 365, "Days since the last genuine contact."), populationKey: population},
		schemaKeyRequired:   []string{daysKey, populationKey}}
}

func QuietCustomerTypes() []string { return []string{"contact", "company"} }

func validateQuietReminderParams(daysKey, populationKey string) func(map[string]any) error {
	return func(params map[string]any) error {
		for key := range params {
			if key != daysKey && key != populationKey {
				return &ParamError{Field: "params." + key, Reason: errNotAParameter}
			}
		}
		if _, present := params[daysKey]; !present {
			return &ParamError{Field: "params." + daysKey, Reason: "choose the quiet period"}
		}
		if err := validateSingleIntParam(daysKey, 365)(map[string]any{daysKey: params[daysKey]}); err != nil {
			return err
		}
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		population, err := quietPopulation(raw)
		if err != nil || !population.configured() {
			return &ParamError{Field: "params." + populationKey, Reason: "choose which records this reminder covers"}
		}
		return nil
	}
}
