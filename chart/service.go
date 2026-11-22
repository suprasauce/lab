package chart

import "optionsbacktest/repository"

type ChartService struct {
	repo       *repository.Repository
	indicators map[string]interface{}
}

func NewChartService(repo *repository.Repository) *ChartService {
	return &ChartService{repo: repo, indicators: make(map[string]interface{})}
}
