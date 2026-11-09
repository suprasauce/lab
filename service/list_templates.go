package service

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"optionsbacktest/repository"
)

type TemplatesListResponse struct {
	Templates []StrategyResponse `json:"templates"`
}

func ListTemplates(ctx context.Context, repo *repository.Repository) (TemplatesListResponse, error) {
	ownerID, err := strconv.ParseInt(os.Getenv("TEMPLATE_OWNER_USER_ID"), 10, 64)
	if err != nil || ownerID <= 0 {
		return TemplatesListResponse{}, fmt.Errorf("TEMPLATE_OWNER_USER_ID must be a positive integer")
	}
	templates, err := repo.ListTemplates(ctx, ownerID)
	if err != nil {
		return TemplatesListResponse{}, err
	}
	response := TemplatesListResponse{Templates: make([]StrategyResponse, len(templates))}
	for i, template := range templates {
		response.Templates[i] = StrategyResponse{
			ID:        strconv.FormatInt(template.ID, 10),
			Name:      template.Name,
			Script:    template.Script,
			CreatedAt: template.CreatedAt,
			UpdatedAt: template.UpdatedAt,
		}
	}
	return response, nil
}
