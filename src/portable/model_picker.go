package main

import "fyne.io/fyne/v2/widget"

const threadModelLabel = "Modèle de la discussion"
const defaultEffortLabel = "Par défaut"

type modelPicker struct {
	model, effort *widget.Select
	models        map[string]catalogModel
	efforts       map[string]string
}

func newModelPicker() *modelPicker {
	picker := &modelPicker{
		model:  widget.NewSelect([]string{threadModelLabel}, nil),
		effort: widget.NewSelect(nil, nil),
	}
	picker.model.OnChanged = picker.updateEfforts
	picker.model.SetSelected(threadModelLabel)
	picker.model.Disable()
	return picker
}

func (picker *modelPicker) setCatalog(models []catalogModel) {
	picker.models = make(map[string]catalogModel, len(models))
	labels := []string{threadModelLabel}
	for _, model := range models {
		label := model.Model
		if model.DisplayName != "" && model.DisplayName != model.Model {
			label = model.DisplayName + " · " + model.Model
		}
		picker.models[label] = model
		labels = append(labels, label)
	}
	picker.model.SetOptions(labels)
	if len(models) > 0 {
		picker.model.Enable()
	}
}

func (picker *modelPicker) updateEfforts(label string) {
	model := picker.models[label]
	picker.efforts = make(map[string]string)
	if model.Model == "" {
		picker.effort.PlaceHolder = "Selon la discussion"
		picker.effort.SetOptions(nil)
		picker.effort.ClearSelected()
		picker.effort.Disable()
		return
	}
	labels := []string{defaultEffortLabel}
	for _, effort := range model.SupportedReasoningEfforts {
		label := effortLabel(effort.ReasoningEffort)
		if label == "" || picker.efforts[label] != "" {
			continue
		}
		picker.efforts[label] = effort.ReasoningEffort
		labels = append(labels, label)
	}
	picker.effort.SetOptions(labels)
	picker.effort.SetSelected(defaultEffortLabel)
	if len(labels) > 1 {
		picker.effort.Enable()
	} else {
		picker.effort.Disable()
	}
}

func (picker *modelPicker) selection() turnOptions {
	return turnOptions{Model: picker.models[picker.model.Selected].Model, Effort: picker.efforts[picker.effort.Selected]}
}

func effortLabel(effort string) string {
	switch effort {
	case "none":
		return "Aucun"
	case "minimal":
		return "Minimal"
	case "low":
		return "Faible"
	case "medium":
		return "Moyen"
	case "high":
		return "Élevé"
	case "xhigh":
		return "Très élevé"
	case "max":
		return "Maximum"
	case "ultra":
		return "Ultra"
	default:
		return effort
	}
}
