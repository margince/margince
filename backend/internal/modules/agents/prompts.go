// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The starter prompts a host shows as slash commands or a menu.
//
// Each one is a request a user would type, written so the model reaches the
// right tool on the first call. They are the jobs the end-to-end lane already
// proves, so a prompt never promises work the surface has not been seen to do.

import (
	"context"
	"encoding/json"
	"strings"
)

// starterPrompt is one entry of prompts/list, and the text prompts/get expands.
type starterPrompt struct {
	Name        string
	Title       string
	Description string
	Arguments   []promptArgument
	// Tools lists the tools the request text names. A caller must be offered all
	// of them to be offered the prompt. A prompt that sends the model to a tool
	// it cannot call fails on its first step.
	Tools []string
	// Text is the user message, with {argument} placeholders.
	Text string
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

var starterPrompts = []starterPrompt{
	{
		Name: "morning_brief", Title: "Brief me for today",
		Description: "This morning's brief, what is slipping and what waits for your decision.",
		Tools:       []string{"read_context", "read_attention", listApprovalsToolName},
		Text: "Brief me for today. Read this morning's brief with read_context (about: brief), " +
			"check read_attention (view: slipping) for deals at risk, and call list_approvals for what " +
			"waits for my decision. Give me the three things to do first, each with its link.",
	},
	{
		Name: "whats_slipping", Title: "What is slipping this week",
		Description: "Deals at risk this week, and why each one is on the list.",
		Tools:       []string{"read_attention"},
		Text: "What is slipping this week? Call read_attention with view slipping, then tell me " +
			"for each deal why it is at risk and the one step that would help, with its link.",
	},
	{
		Name: "prepare_meeting", Title: "Prepare my next meeting",
		Description: "Who is coming, the open threads and what was promised.",
		Arguments:   []promptArgument{{Name: "with", Description: "The contact, company or deal the meeting is about", Required: true}},
		Tools:       []string{"read_context"},
		Text: "Prepare me for my meeting with {with}. Call read_context with about meeting and " +
			"record_name {with}. Tell me who is coming, the open threads and what we promised.",
	},
	{
		Name: "catch_up", Title: "Catch me up on a record",
		Description: "What has been happening on one contact, company, deal or project.",
		Arguments:   []promptArgument{{Name: "record", Description: "The name of the contact, company, deal or project", Required: true}},
		Tools:       []string{"read_context"},
		Text: "Catch me up on {record}. Call read_context with about catch_up and record_name " +
			"{record}, and tell me what changed and what is next.",
	},
	{
		Name: "open_promises", Title: "What did we promise customers",
		Description: "Promises made to customers that are still open.",
		Tools:       []string{"read_attention"},
		Text: "What have we promised customers that is still open? Call read_attention with view " +
			"commitments, and list each promise with who owns it, when it is due and its link.",
	},
	{
		Name: "explain_forecast", Title: "Explain my forecast",
		Description: "The forecast for this period and what moved it.",
		Tools:       []string{"read_forecast"},
		Text: "Explain my forecast. Call read_forecast for this period and how it moved, then tell " +
			"me in plain words what changed and which deals moved it.",
	},
	{
		Name: "log_call", Title: "Log a call",
		Description: "Log a call on the record it was about.",
		Arguments: []promptArgument{
			{Name: "with", Description: "Who the call was with", Required: true},
			{Name: "notes", Description: "What was said and agreed", Required: true},
		},
		Tools: []string{"find_records", "change_activities"},
		Text: "Log a call with {with}. Find the record with find_records, then log the call with " +
			"change_activities. My notes: {notes}",
	},
}

// offeredPrompts are the starter prompts whose every tool this caller is
// offered, in catalog order.
func (s *Dispatcher) offeredPrompts(ctx context.Context) []starterPrompt {
	offered := map[string]bool{}
	for _, spec := range s.registry.Offered(ctx) {
		offered[spec.Name] = true
	}
	var out []starterPrompt
	for _, prompt := range starterPrompts {
		if allOffered(prompt.Tools, offered) {
			out = append(out, prompt)
		}
	}
	return out
}

func allOffered(tools []string, offered map[string]bool) bool {
	for _, name := range tools {
		if !offered[name] {
			return false
		}
	}
	return true
}

// promptList answers prompts/list.
func (s *Dispatcher) promptList(ctx context.Context) []map[string]any {
	prompts := s.offeredPrompts(ctx)
	out := make([]map[string]any, 0, len(prompts))
	for _, prompt := range prompts {
		entry := map[string]any{fieldName: prompt.Name, fieldTitle: prompt.Title, "description": prompt.Description}
		if len(prompt.Arguments) > 0 {
			entry["arguments"] = prompt.Arguments
		}
		out = append(out, entry)
	}
	return out
}

// promptGet answers prompts/get: the request text with the caller's arguments in it.
func (s *Dispatcher) promptGet(ctx context.Context, params json.RawMessage) (map[string]any, *rpcError) {
	var p struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, invalidPromptParams(`prompts/get takes {"name": string, "arguments": {string: string}}`)
	}
	for _, prompt := range s.offeredPrompts(ctx) {
		if prompt.Name != p.Name {
			continue
		}
		text := prompt.Text
		for _, arg := range prompt.Arguments {
			value := strings.TrimSpace(p.Arguments[arg.Name])
			if value == "" && arg.Required {
				return nil, invalidPromptParams("prompt " + prompt.Name + " needs the argument " + arg.Name + ": " + arg.Description)
			}
			text = strings.ReplaceAll(text, "{"+arg.Name+"}", value)
		}
		return map[string]any{
			"description": prompt.Description,
			"messages": []map[string]any{{
				"role":    "user",
				"content": map[string]any{"type": fieldText, fieldText: text},
			}},
		}, nil
	}
	return nil, invalidPromptParams("no prompt is named " + p.Name + "; prompts/list names the ones you are offered")
}

func invalidPromptParams(message string) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: "invalid params: " + message}
}

// StarterPromptNames is the whole catalog, for the composition's census that
// a caller holding every scope is offered every prompt.
func StarterPromptNames() []string {
	names := make([]string, 0, len(starterPrompts))
	for _, prompt := range starterPrompts {
		names = append(names, prompt.Name)
	}
	return names
}
