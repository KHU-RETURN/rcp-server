package admin

import (
	"context"
	"strconv"
	"time"
)

const (
	defaultPage  = 1
	defaultLimit = 10
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

func (s *Service) Users(ctx context.Context, rawPage, rawLimit string) (PaginatedUsersResponse, error) {
	return s.repo.Users(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) Instances(ctx context.Context, rawPage, rawLimit string) (PaginatedInstancesResponse, error) {
	return s.repo.Instances(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) Containers(ctx context.Context, rawPage, rawLimit string) (PaginatedContainersResponse, error) {
	return s.repo.Containers(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) UserResources(ctx context.Context, id string) (UserResourcesResponse, error) {
	return s.repo.UserResources(ctx, id)
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

func parsePageParams(rawPage, rawLimit string) PageParams {
	page := parsePositiveInt(rawPage, defaultPage)
	limit := parsePositiveInt(rawLimit, defaultLimit)
	if limit > maxLimit {
		limit = maxLimit
	}
	return PageParams{Page: page, Limit: limit}
}

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
