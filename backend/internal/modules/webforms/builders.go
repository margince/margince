// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"encoding/json"
	"strings"
)

// Each builder posts its own JSON shape, and these read the shape it documents
// for its form webhook. A field the builder typed as an email or phone number
// is read as one, whatever its label says.

type webflowBody struct {
	TriggerType string `json:"triggerType"` //nolint:tagliatelle // Webflow's wire format (camelCase)
	Payload     struct {
		ID   string                     `json:"id"`
		Data map[string]json.RawMessage `json:"data"`
	} `json:"payload"`
}

func webflowPayload(body []byte) (builderPayload, error) {
	var in webflowBody
	if err := json.Unmarshal(body, &in); err != nil || in.TriggerType != "form_submission" {
		return builderPayload{}, errUnreadable
	}
	values := make(map[string]string, len(in.Payload.Data))
	for name, raw := range in.Payload.Data {
		values[name] = scalarText(raw)
	}
	return builderPayload{Fields: sortedFields(values), ID: in.Payload.ID}, nil
}

type typeformBody struct {
	EventID      string `json:"event_id"`
	FormResponse struct {
		Token      string `json:"token"`
		Definition struct {
			Fields []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"fields"`
		} `json:"definition"`
		Answers []typeformAnswer  `json:"answers"`
		Hidden  map[string]string `json:"hidden"`
	} `json:"form_response"`
}

type typeformAnswer struct {
	Type  string `json:"type"`
	Field struct {
		ID  string `json:"id"`
		Ref string `json:"ref"`
	} `json:"field"`
	Text        *string          `json:"text"`
	Email       *string          `json:"email"`
	PhoneNumber *string          `json:"phone_number"`
	URL         *string          `json:"url"`
	Date        *string          `json:"date"`
	Number      *json.RawMessage `json:"number"`
	Boolean     *bool            `json:"boolean"`
	Choice      *struct {
		Label string `json:"label"`
		Other string `json:"other"`
	} `json:"choice"`
	Choices *struct {
		Labels []string `json:"labels"`
		Other  string   `json:"other"`
	} `json:"choices"`
}

func (a typeformAnswer) text() string {
	for _, s := range []*string{a.Text, a.Email, a.PhoneNumber, a.URL, a.Date} {
		if s != nil {
			return *s
		}
	}
	switch {
	case a.Number != nil:
		return string(*a.Number)
	case a.Boolean != nil && *a.Boolean:
		return "true"
	case a.Boolean != nil:
		return "false"
	case a.Choice != nil:
		return strings.TrimSpace(a.Choice.Label + " " + a.Choice.Other)
	case a.Choices != nil:
		labels := a.Choices.Labels
		if a.Choices.Other != "" {
			labels = append(labels, a.Choices.Other)
		}
		return strings.Join(labels, ", ")
	}
	return ""
}

func typeformPayload(body []byte) (builderPayload, error) {
	var in typeformBody
	if err := json.Unmarshal(body, &in); err != nil || in.EventID == "" {
		return builderPayload{}, errUnreadable
	}
	titles := make(map[string]string, len(in.FormResponse.Definition.Fields))
	for _, f := range in.FormResponse.Definition.Fields {
		titles[f.ID] = f.Title
	}
	var out []field
	for _, a := range in.FormResponse.Answers {
		name := titles[a.Field.ID]
		switch {
		case a.Type == "email":
			name = fieldEmail
		case a.Type == "phone_number":
			name = fieldPhone
		case name == "":
			name = a.Field.Ref
		}
		out = append(out, field{Name: name, Value: a.text()})
	}
	out = append(out, sortedFields(in.FormResponse.Hidden)...)
	return builderPayload{Fields: out, ID: in.EventID}, nil
}

type tallyBody struct {
	EventID   string `json:"eventId"`   //nolint:tagliatelle // Tally's wire format (camelCase)
	EventType string `json:"eventType"` //nolint:tagliatelle // Tally's wire format (camelCase)
	Data      struct {
		Fields []struct {
			Key     string          `json:"key"`
			Label   string          `json:"label"`
			Type    string          `json:"type"`
			Value   json.RawMessage `json:"value"`
			Options []tallyOption   `json:"options"`
		} `json:"fields"`
	} `json:"data"`
}

type tallyOption struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func tallyPayload(body []byte) (builderPayload, error) {
	var in tallyBody
	if err := json.Unmarshal(body, &in); err != nil || in.EventType != "FORM_RESPONSE" {
		return builderPayload{}, errUnreadable
	}
	var out []field
	for _, f := range in.Data.Fields {
		name := f.Label
		switch {
		case f.Type == "INPUT_EMAIL":
			name = fieldEmail
		case f.Type == "INPUT_PHONE_NUMBER":
			name = fieldPhone
		case name == "":
			name = f.Key
		}
		value := scalarText(f.Value)
		var chosen []string
		if json.Unmarshal(f.Value, &chosen) == nil && len(f.Options) > 0 {
			value = strings.Join(optionTexts(chosen, f.Options), ", ")
		}
		out = append(out, field{Name: name, Value: value})
	}
	return builderPayload{Fields: out, ID: in.EventID}, nil
}

func optionTexts(chosen []string, options []tallyOption) []string {
	texts := make([]string, 0, len(chosen))
	for _, id := range chosen {
		for _, o := range options {
			if o.ID == id {
				texts = append(texts, o.Text)
			}
		}
	}
	return texts
}
