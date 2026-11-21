package chart

import "optionsbacktest/repository"

type ChartService struct {
	repo *repository.Repository
}

func NewChartService(repo *repository.Repository) *ChartService {
	return &ChartService{repo: repo}
}
