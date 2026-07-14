package admin

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultPage  = 1
	defaultLimit = 10
	maxLimit     = 500
)

type Service struct {
	repo         *Repository
	health       healthChecker
	statusSource instanceStatusSource
}

func NewService(repo *Repository, health healthChecker, statusSource instanceStatusSource) *Service {
	return &Service{repo: repo, health: health, statusSource: statusSource}
}

func (s *Service) Summary(ctx context.Context) (SummaryResponse, error) {
	counts, err := s.repo.SummaryCounts(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}

	// The DB status is written once at creation and goes stale, so count from
	// live OpenStack statuses and fall back to the DB status only for
	// instances missing from the live map — same policy as overlayLiveStatuses.
	live := s.liveStatuses(ctx)
	statusCounts := make(map[string]int)
	for _, row := range counts.Instances {
		status := row.Status
		if liveStatus, ok := live[row.ID]; ok && liveStatus != "" {
			status = liveStatus
		}
		status = strings.ToUpper(strings.TrimSpace(status))
		if status == "" {
			status = "UNKNOWN"
		}
		statusCounts[status]++
	}

	return SummaryResponse{
		Users:        counts.Users,
		Instances:    len(counts.Instances),
		Containers:   counts.Containers,
		Keypairs:     counts.Keypairs,
		StatusCounts: statusCounts,
	}, nil
}

func (s *Service) Users(ctx context.Context, rawPage, rawLimit string) (PaginatedUsersResponse, error) {
	return s.repo.Users(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) Instances(ctx context.Context, rawPage, rawLimit string) (PaginatedInstancesResponse, error) {
	res, err := s.repo.Instances(ctx, parsePageParams(rawPage, rawLimit))
	if err != nil {
		return PaginatedInstancesResponse{}, err
	}
	s.overlayLiveStatuses(ctx, res.Items)
	return res, nil
}

func (s *Service) Instance(ctx context.Context, id string) (InstanceResponse, error) {
	res, err := s.repo.Instance(ctx, id)
	if err != nil {
		return InstanceResponse{}, err
	}
	if s.statusSource != nil {
		if live, statusErr := s.statusSource.InstanceStatus(ctx, res.ID); statusErr == nil && live != "" {
			res.Status = live
		}
	}
	return res, nil
}

func (s *Service) Containers(ctx context.Context, rawPage, rawLimit string) (PaginatedContainersResponse, error) {
	return s.repo.Containers(ctx, parsePageParams(rawPage, rawLimit))
}

func (s *Service) Container(ctx context.Context, id string) (ContainerResponse, error) {
	return s.repo.Container(ctx, id)
}

func (s *Service) UserResources(ctx context.Context, id string) (UserResourcesResponse, error) {
	res, err := s.repo.UserResources(ctx, id)
	if err != nil {
		return UserResourcesResponse{}, err
	}
	s.overlayLiveStatuses(ctx, res.Instances)
	return res, nil
}

// overlayLiveStatuses replaces stale DB statuses with live OpenStack statuses, best-effort.
func (s *Service) overlayLiveStatuses(ctx context.Context, items []InstanceResponse) {
	if len(items) == 0 {
		return
	}
	statuses := s.liveStatuses(ctx)
	for i := range items {
		if live, ok := statuses[items[i].ID]; ok && live != "" {
			items[i].Status = live
		}
	}
}

// liveStatuses fetches the live OpenStack status map, best-effort; nil when unavailable.
func (s *Service) liveStatuses(ctx context.Context) map[string]string {
	if s.statusSource == nil {
		return nil
	}
	statuses, err := s.statusSource.InstanceStatuses(ctx)
	if err != nil {
		return nil
	}
	return statuses
}

func (s *Service) System(ctx context.Context) SystemResponse {
	openstackStatus := "unconfigured"
	storageStatus := "unconfigured"
	sshGatewayStatus := "unconfigured"
	nsProxyStatus := "unconfigured"
	httpProxyStatus := "unconfigured"
	if s.health != nil {
		// Each check has its own timeout; run them concurrently so the
		// endpoint responds in one timeout's worth of time, not five.
		var wg sync.WaitGroup
		run := func(target *string, check func(context.Context) error) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				*target = healthStatus(check(ctx))
			}()
		}
		run(&openstackStatus, s.health.CheckOpenStack)
		run(&storageStatus, s.health.CheckStorage)
		run(&sshGatewayStatus, s.health.CheckSSHGateway)
		run(&nsProxyStatus, s.health.CheckNSProxy)
		run(&httpProxyStatus, s.health.CheckHTTPProxy)
		wg.Wait()
	}

	return SystemResponse{
		APIStatus:        "healthy",
		OpenStackStatus:  openstackStatus,
		SSHGatewayStatus: sshGatewayStatus,
		NSProxyStatus:    nsProxyStatus,
		HTTPProxyStatus:  httpProxyStatus,
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
