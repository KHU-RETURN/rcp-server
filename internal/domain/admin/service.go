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
	repo   *Repository
	health healthChecker
}

func NewService(repo *Repository, health healthChecker) *Service {
	return &Service{repo: repo, health: health}
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

func (s *Service) Instance(ctx context.Context, id string) (InstanceResponse, error) {
	return s.repo.Instance(ctx, id)
}

func (s *Service) Containers(ctx context.Context, rawPage, rawLimit string) (PaginatedContainersResponse, error) {
	return s.repo.Containers(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) Container(ctx context.Context, id string) (ContainerResponse, error) {
	return s.repo.Container(ctx, id)
}

func (s *Service) UserResources(ctx context.Context, id string) (UserResourcesResponse, error) {
	return s.repo.UserResources(ctx, id)
}

func (s *Service) System(ctx context.Context) SystemResponse {
	openstackStatus := "unconfigured"
	storageStatus := "unconfigured"
	sshGatewayStatus := "unconfigured"
	if s.health != nil {
		openstackStatus = healthStatus(s.health.CheckOpenStack(ctx))
		storageStatus = healthStatus(s.health.CheckStorage(ctx))
		sshGatewayStatus = healthStatus(s.health.CheckSSHGateway(ctx))
	}

	return SystemResponse{
		APIStatus:        "healthy",
		OpenStackStatus:  openstackStatus,
		SSHGatewayStatus: sshGatewayStatus,
		StorageStatus:    storageStatus,
		LastUpdatedAt:    time.Now().UTC(),
		Message:          "System status is checked from the API server against configured providers.",
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
