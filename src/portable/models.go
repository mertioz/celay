package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type catalogModel struct {
	Model, DefaultReasoningEffort string
	Hidden, IsDefault             bool
	InputModalities               []string
}

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
func resolveTurnModel(ctx context.Context, call func(context.Context, string, any, any) error,
	provider, saved string) (catalogModel, error) {
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

	var fallback catalogModel
	for cursor := ""; ; {
		params := map[string]any{"includeHidden": false, "limit": 50}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page modelPage
		if err := call(ctx, "model/list", params, &page); err != nil {
			return catalogModel{}, fmt.Errorf("Impossible de lire les modèles du Codex installé. %w", err)
		}
		for _, model := range page.Data {
			if model.Model == "" || model.Hidden ||
				(len(model.InputModalities) > 0 && !slices.Contains(model.InputModalities, "text")) {
				continue
			}
			if model.Model == saved {
				return catalogModel{}, nil
			}
			if fallback.Model == "" || (model.IsDefault && !fallback.IsDefault) {
				fallback = model
			}
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			break
		}
		cursor = page.NextCursor
	}
	if fallback.Model == "" {
		return catalogModel{}, errors.New("Le Codex installé ne propose aucun modèle de texte pour ce compte.")
	}
	return fallback, nil
}

func turnDetail(turnID, detail string) string {
	if turnID != "" && (strings.Contains(detail, "model_not_found") ||
		strings.Contains(detail, "model is not supported when using Codex with a ChatGPT account")) {
		return "Le message figure dans la discussion. Codex a refusé le modèle utilisé pour répondre."
	}
	return detail
}
