package admin

import (
	"context"
	"strconv"
	"time"
)

const (
	defaultLimit = 100
	maxLimit     = 500
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Summary(ctx context.Context) (SummaryResponse, error) {
	return s.repo.Summary(ctx)
}

func (s *Service) Users(ctx context.Context, rawLimit string) ([]UserResponse, error) {
	return s.repo.Users(ctx, parseLimit(rawLimit))
}

func (s *Service) Instances(ctx context.Context, rawLimit string) ([]InstanceResponse, error) {
	return s.repo.Instances(ctx, parseLimit(rawLimit))
}

func (s *Service) System() SystemResponse {
	return SystemResponse{
		APIStatus:        "healthy",
		OpenStackStatus:  "unknown",
		SSHGatewayStatus: "unknown",
		StorageStatus:    "unknown",
		LastUpdatedAt:    time.Now().UTC(),
		Message:          "System status is limited to API health until provider health checks are wired.",
	}
}

func parseLimit(raw string) int {
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
