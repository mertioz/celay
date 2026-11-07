package main

import (
	"context"
	"encoding/json"
	"testing"
)

func TestResolveTurnModel(t *testing.T) {
	const saved = "gpt-6.1-sol"
	const account = `{"account":{"type":"chatgpt"},"requiresOpenaiAuth":true}`
	const olderRuntime = `{"data":[
		{"model":"gpt-6-sol","inputModalities":["text"]},
		{"model":"gpt-6-astra","isDefault":true,"defaultReasoningEffort":"medium","inputModalities":["text"]}]}`
	tests := []struct {
		name, provider, auth, wantModel, wantEffort string
		pages                                       []string
		wantError                                   bool
	}{
		{name: "older runtime uses its default", provider: "openai", auth: account,
			pages: []string{olderRuntime}, wantModel: "gpt-6-astra", wantEffort: "medium"},
		{name: "installed Desktop keeps the saved model", provider: "openai", auth: account,
			pages: []string{`{"data":[{"model":"gpt-6.1-sol","inputModalities":["text"]}]}`}},
		{name: "saved model on the next page", provider: "openai", auth: account,
			pages: []string{`{"data":[{"model":"other","isDefault":true}],"nextCursor":"next"}`,
				`{"data":[{"model":"gpt-6.1-sol"}]}`}},
		{name: "custom provider keeps its settings", provider: "custom"},
		{name: "API key keeps its configured model", provider: "openai",
			auth: `{"account":{"type":"apiKey"},"requiresOpenaiAuth":true}`},
		{name: "account required before adding a message", provider: "openai", wantError: true,
			auth: `{"account":null,"requiresOpenaiAuth":true}`},
		{name: "no available text model", provider: "openai", auth: account, wantError: true,
			pages: []string{`{"data":[{"model":"hidden","hidden":true},{"model":"image-only","inputModalities":["image"]}]}`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			accountCalls, modelCalls := 0, 0
			call := func(_ context.Context, method string, params, result any) error {
				switch method {
				case "account/read":
					accountCalls++
					return json.Unmarshal([]byte(test.auth), result)
				case "model/list":
					if modelCalls >= len(test.pages) {
						t.Fatal("unexpected model/list call")
					}
					if modelCalls > 0 && params.(map[string]any)["cursor"] != "next" {
						t.Fatal("model/list cursor was not propagated")
					}
					modelCalls++
					return json.Unmarshal([]byte(test.pages[modelCalls-1]), result)
				default:
					t.Fatalf("unexpected RPC method: %s", method)
					return nil
				}
			}
			model, err := resolveTurnModel(context.Background(), call, test.provider, saved)
			if (err != nil) != test.wantError || model.Model != test.wantModel ||
				model.DefaultReasoningEffort != test.wantEffort {
				t.Fatalf("got model %q, effort %q, error %v", model.Model, model.DefaultReasoningEffort, err)
			}
			if modelCalls != len(test.pages) || (accountCalls == 0) != (test.auth == "") {
				t.Fatalf("unexpected preflight calls: %d accounts, %d model pages", accountCalls, modelCalls)
			}
		})
	}
}

func TestUnsupportedModelDetailKeepsDeliveryDistinction(t *testing.T) {
	const upstream = `{"error":{"code":"model_not_found"}}`
	if got := turnDetail("accepted-turn", upstream); got == upstream {
		t.Fatal("an accepted turn needs a clear explanation of the response failure")
	}
	if got := turnDetail("", upstream); got != upstream {
		t.Fatal("without a turn ID the UI must not claim that the prompt is in the discussion")
	}
}
