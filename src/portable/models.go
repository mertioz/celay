package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type turnOptions struct {
	Model, Effort string
}

type catalogModel struct {
	Model, DisplayName, DefaultReasoningEffort string
	Hidden, IsDefault                          bool
	InputModalities                            []string
	SupportedReasoningEfforts                  []struct{ ReasoningEffort string }
}

type rpcCall func(context.Context, string, any, any) error

type codexAccount struct {
	Account            *struct{ Type string }
	RequiresOpenaiAuth bool
}

type modelPage struct {
	Data       []catalogModel
	NextCursor string
}

// A thread can contain a model saved by a newer Codex runtime. Check the
// sending runtime's catalog before appending the user's message.
func resolveTurnModel(ctx context.Context, call rpcCall, provider, saved string) (catalogModel, error) {
	if provider != "openai" || saved == "" {
		return catalogModel{}, nil
	}
	var account codexAccount
	if err := call(ctx, "account/read", map[string]bool{"refreshToken": false}, &account); err != nil {
		return catalogModel{}, fmt.Errorf("Impossible de vérifier le compte Codex. %w", err)
	}
	if account.Account == nil {
		if account.RequiresOpenaiAuth {
			return catalogModel{}, errors.New("Connecte Codex à ton compte avant de programmer un envoi.")
		}
		return catalogModel{}, nil
	}
	if !strings.HasPrefix(account.Account.Type, "chatgpt") {
		return catalogModel{}, nil
	}

	models, err := listTextModels(ctx, call)
	if err != nil {
		return catalogModel{}, err
	}
	var fallback catalogModel
	for _, model := range models {
		if model.Model == saved {
			return catalogModel{}, nil
		}
		if fallback.Model == "" || (model.IsDefault && !fallback.IsDefault) {
			fallback = model
		}
	}
	if fallback.Model == "" {
		return catalogModel{}, errors.New("Le Codex installé ne propose aucun modèle de texte pour ce compte.")
	}
	return fallback, nil
}

func resolveTurnOptions(ctx context.Context, call rpcCall, provider, saved string, selected turnOptions) (turnOptions, error) {
	if selected.Model == "" {
		model, err := resolveTurnModel(ctx, call, provider, saved)
		return turnOptions{Model: model.Model, Effort: model.DefaultReasoningEffort}, err
	}
	models, err := listTextModels(ctx, call)
	if err != nil {
		return turnOptions{}, err
	}
	return validateTurnOptions(models, selected)
}

func validateTurnOptions(models []catalogModel, selected turnOptions) (turnOptions, error) {
	for _, model := range models {
		if model.Model != selected.Model {
			continue
		}
		if selected.Effort == "" {
			selected.Effort = model.DefaultReasoningEffort
			return selected, nil
		}
		for _, effort := range model.SupportedReasoningEfforts {
			if effort.ReasoningEffort == selected.Effort {
				return selected, nil
			}
		}
		return turnOptions{}, fmt.Errorf("L’effort de raisonnement « %s » n’est plus proposé pour %s.", selected.Effort, selected.Model)
	}
	return turnOptions{}, fmt.Errorf("Le modèle « %s » choisi pour cet envoi n’est plus proposé par Codex.", selected.Model)
}

func listTextModels(ctx context.Context, call rpcCall) ([]catalogModel, error) {
	var models []catalogModel
	seenModels := make(map[string]bool)
	seenCursors := make(map[string]bool)
	for cursor := ""; ; {
		params := map[string]any{"includeHidden": false, "limit": 50}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page modelPage
		if err := call(ctx, "model/list", params, &page); err != nil {
			return nil, fmt.Errorf("Impossible de lire les modèles du Codex installé. %w", err)
		}
		for _, model := range page.Data {
			if model.Model == "" || model.Hidden || seenModels[model.Model] ||
				(len(model.InputModalities) > 0 && !slices.Contains(model.InputModalities, "text")) {
				continue
			}
			seenModels[model.Model] = true
			models = append(models, model)
		}
		if page.NextCursor == "" || seenCursors[page.NextCursor] {
			break
		}
		seenCursors[page.NextCursor] = true
		cursor = page.NextCursor
	}
	return models, nil
}

func turnDetail(turnID, detail string) string {
	if turnID != "" && (strings.Contains(detail, "model_not_found") ||
		strings.Contains(detail, "model is not supported when using Codex with a ChatGPT account")) {
		return "Le message figure dans la discussion. Codex a refusé le modèle utilisé pour répondre."
	}
	return detail
}
