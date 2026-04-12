package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// computeClient는 OpenStack compute API 접근 인터페이스입니다.
// 구현체는 client.go의 Client입니다.
type computeClient interface {
	FetchFlavors() ([]Flavor, error)
	GetComputeQuota(projectID string) (*QuotaDetailSet, error)
	CreateServer(opts CreateServerOpts) (*Server, error)
	DeleteServer(id string) error
	FetchInstances() ([]Server, error)
	FetchInstance(id string) (*Server, error)
	FetchDiagnostics(id string) (map[string]any, error)
}

// instanceRepo는 인스턴스 영속성 인터페이스입니다.
// 구현체는 repository.go의 Repository입니다.
type instanceRepo interface {
	SaveInstance(ctx context.Context, ownerID uuid.UUID, inst *Instance) error
	DeleteByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) error
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Instance, error)
	FindByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) (*Instance, error)
}

// Service는 비즈니스 로직을 담당합니다.
type Service struct {
	client    computeClient
	repo      instanceRepo
	projectID string
}

var (
	ErrCreateInstanceNameRequired   = errors.New("name is required")
	ErrCreateInstanceImageRequired  = errors.New("image_id is required")
	ErrCreateInstanceFlavorRequired = errors.New("flavor_id is required")
	ErrInstanceNotFound             = errors.New("instance not found")
	ErrInstanceOperationFailed      = errors.New("instance operation failed")
)

// NewService는 새로운 서비스를 생성합니다.
func NewService(client computeClient, repo instanceRepo, projectID string) *Service {
	return &Service{client: client, repo: repo, projectID: projectID}
}

// GetFlavors는 Repo에서 가져온 데이터를 우리 규격(FlavorResponse)으로 변환합니다.
func (s *Service) GetFlavors() ([]FlavorResponse, error) {
	rawFlavors, err := s.client.FetchFlavors()
	if err != nil {
		return nil, err
	}

	var res []FlavorResponse
	for _, f := range rawFlavors {
		res = append(res, FlavorResponse(f))
	}
	return res, nil
}

// GetAvailableFlavorsWithLimit는 남은 자원량을 계산하여 각 Flavor별 가용 대수를 포함해 반환합니다.
func (s *Service) GetAvailableFlavorsWithLimit() ([]AvailableFlavorResponse, error) {
	rawFlavors, err := s.client.FetchFlavors()
	if err != nil {
		return nil, err
	}

	quota, err := s.client.GetComputeQuota(s.projectID)
	if err != nil {
		return nil, fmt.Errorf("쿼터 조회 실패: %v", err)
	}

	remCores := quota.Cores.Limit - quota.Cores.InUse
	remRAM := quota.RAM.Limit - quota.RAM.InUse
	remInstances := quota.Instances.Limit - quota.Instances.InUse

	var res []AvailableFlavorResponse
	for _, f := range rawFlavors {
		countByCPU := 0
		if f.VCPUs > 0 {
			countByCPU = remCores / f.VCPUs
		} else {
			countByCPU = 999
		}

		countByRAM := 0
		if f.RAM > 0 {
			countByRAM = remRAM / f.RAM
		} else {
			countByRAM = 999
		}

		maxPossible := max(min(remInstances, min(countByRAM, countByCPU)), 0)

		res = append(res, AvailableFlavorResponse{
			FlavorResponse:  FlavorResponse(f),
			MaxConfigurable: maxPossible,
		})
	}
	return res, nil
}

// GetInstances는 DB(소유권)와 OpenStack(가변 상태)을 병렬로 읽어 조합하여 반환합니다.
func (s *Service) GetInstances(ctx context.Context, ownerID uuid.UUID) ([]InstanceDetailResponse, error) {
	var (
		dbInstances          []Instance
		osServers            []Server
		flavorMap            map[string]FlavorResponse
		dbErr, osErr, flvErr error
	)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		dbInstances, dbErr = s.repo.ListByOwner(ctx, ownerID)
	}()
	go func() {
		defer wg.Done()
		osServers, osErr = s.client.FetchInstances()
	}()
	go func() {
		defer wg.Done()
		flavorMap, flvErr = s.fetchFlavorMap()
	}()
	wg.Wait()

	if dbErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, dbErr)
	}
	if osErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, osErr)
	}
	if flvErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, flvErr)
	}

	serverMap := make(map[string]Server, len(osServers))
	for _, srv := range osServers {
		serverMap[srv.ID] = srv
	}

	res := make([]InstanceDetailResponse, 0, len(dbInstances))
	for _, inst := range dbInstances {
		srv := serverMap[inst.OpenstackID]
		fixedIP, floatingIP := extractServerIPs(&srv)
		res = append(res, InstanceDetailResponse{
			ID:         inst.OpenstackID,
			Name:       inst.Name,
			Status:     srv.Status,
			Image:      inst.ImageID,
			Flavor:     flavorMap[inst.FlavorID],
			FixedIP:    fixedIP,
			FloatingIP: floatingIP,
			Created:    inst.Created,
		})
	}
	return res, nil
}

// GetInstanceDetail은 DB(소유권), OpenStack 상태, diagnostics를 병렬로 읽어 조합하여 반환합니다.
func (s *Service) GetInstanceDetail(ctx context.Context, ownerID uuid.UUID, id string) (*InstanceDetailResponse, error) {
	var (
		inst                      *Instance
		srv                       *Server
		diag                      map[string]any
		flavorMap                 map[string]FlavorResponse
		dbErr, osErr, flvErr error
	)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		inst, dbErr = s.repo.FindByOpenstackID(ctx, ownerID, id)
	}()
	go func() {
		defer wg.Done()
		srv, osErr = s.client.FetchInstance(id)
	}()
	go func() {
		defer wg.Done()
		// diagnostics 실패는 non-fatal — 사용량 미표시로 처리
		diag, _ = s.client.FetchDiagnostics(id)
	}()
	go func() {
		defer wg.Done()
		flavorMap, flvErr = s.fetchFlavorMap()
	}()
	wg.Wait()

	if dbErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, dbErr)
	}
	if inst == nil {
		return nil, ErrInstanceNotFound
	}
	if osErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, osErr)
	}
	if flvErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, flvErr)
	}

	fixedIP, floatingIP := extractServerIPs(srv)
	return &InstanceDetailResponse{
		ID:         inst.OpenstackID,
		Name:       inst.Name,
		Status:     srv.Status,
		Image:      inst.ImageID,
		Flavor:     flavorMap[inst.FlavorID],
		FixedIP:    fixedIP,
		FloatingIP: floatingIP,
		Usage:      extractUsageStats(diag),
		Created:    inst.Created,
	}, nil
}

// DeleteInstance는 owner 소유 인스턴스를 OpenStack과 DB에서 삭제합니다.
func (s *Service) DeleteInstance(ctx context.Context, ownerID uuid.UUID, id string) error {
	inst, err := s.repo.FindByOpenstackID(ctx, ownerID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	if inst == nil {
		return ErrInstanceNotFound
	}

	if err := s.client.DeleteServer(id); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}

	if err := s.repo.DeleteByOpenstackID(ctx, ownerID, id); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	return nil
}

func (s *Service) CreateInstance(ctx context.Context, ownerID uuid.UUID, opts CreateServerOpts) (*CreateInstanceResponse, error) {
	normalizedOpts := normalizeCreateServerOpts(opts)
	if err := validateCreateServerOpts(normalizedOpts); err != nil {
		return nil, err
	}

	server, err := s.client.CreateServer(normalizedOpts)
	if err != nil {
		return nil, err
	}

	inst := &Instance{
		OpenstackID: server.ID,
		Name:        firstNonEmpty(strings.TrimSpace(server.Name), normalizedOpts.Name),
		ImageID:     firstNonEmpty(extractResourceID(server.Image), normalizedOpts.ImageRef),
		FlavorID:    firstNonEmpty(extractResourceID(server.Flavor), normalizedOpts.FlavorRef),
		Created:     server.Created,
	}

	if err := s.repo.SaveInstance(ctx, ownerID, inst); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}

	return buildCreateInstanceResponse(server, normalizedOpts), nil
}

func (s *Service) fetchFlavorMap() (map[string]FlavorResponse, error) {
	rawFlavors, err := s.client.FetchFlavors()
	if err != nil {
		return nil, err
	}
	m := make(map[string]FlavorResponse, len(rawFlavors))
	for _, f := range rawFlavors {
		m[f.ID] = FlavorResponse(f)
	}
	return m, nil
}

type serverAddress struct {
	Address string `json:"addr"`
	Type    string `json:"OS-EXT-IPS:type"`
}

func buildCreateServerOpts(req CreateInstanceRequest) (CreateServerOpts, error) {
	req = normalizeCreateInstanceRequest(req)

	opts := CreateServerOpts{
		Name:           req.Name,
		ImageRef:       req.ImageID,
		FlavorRef:      req.FlavorID,
		KeyName:        req.KeyName,
		SecurityGroups: req.SecurityGroups,
	}

	if req.NetworkID != "" {
		opts.Networks = []NetworkID{{UUID: req.NetworkID}}
	}

	if err := validateCreateServerOpts(opts); err != nil {
		return CreateServerOpts{}, err
	}

	return opts, nil
}

func normalizeCreateInstanceRequest(req CreateInstanceRequest) CreateInstanceRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.ImageID = strings.TrimSpace(req.ImageID)
	req.FlavorID = strings.TrimSpace(req.FlavorID)
	req.NetworkID = strings.TrimSpace(req.NetworkID)
	req.KeyName = strings.TrimSpace(req.KeyName)
	req.SecurityGroups = normalizeStringSlice(req.SecurityGroups)
	return req
}

func normalizeCreateServerOpts(opts CreateServerOpts) CreateServerOpts {
	opts.Name = strings.TrimSpace(opts.Name)
	opts.ImageRef = strings.TrimSpace(opts.ImageRef)
	opts.FlavorRef = strings.TrimSpace(opts.FlavorRef)
	opts.KeyName = strings.TrimSpace(opts.KeyName)
	opts.SecurityGroups = normalizeStringSlice(opts.SecurityGroups)

	networks := make([]NetworkID, 0, len(opts.Networks))
	for _, network := range opts.Networks {
		network.UUID = strings.TrimSpace(network.UUID)
		if network.UUID == "" {
			continue
		}
		networks = append(networks, network)
	}
	opts.Networks = networks

	return opts
}

func validateCreateServerOpts(opts CreateServerOpts) error {
	switch {
	case strings.TrimSpace(opts.Name) == "":
		return ErrCreateInstanceNameRequired
	case strings.TrimSpace(opts.ImageRef) == "":
		return ErrCreateInstanceImageRequired
	case strings.TrimSpace(opts.FlavorRef) == "":
		return ErrCreateInstanceFlavorRequired
	default:
		return nil
	}
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func buildCreateInstanceResponse(server *Server, opts CreateServerOpts) *CreateInstanceResponse {
	fixedIP, floatingIP := extractServerIPs(server)
	keyName := firstNonEmpty(strings.TrimSpace(server.KeyName), opts.KeyName)
	securityGroups := extractSecurityGroupNames(server.SecurityGroups, opts.SecurityGroups)

	return &CreateInstanceResponse{
		ID:             server.ID,
		Name:           firstNonEmpty(strings.TrimSpace(server.Name), opts.Name),
		Status:         strings.TrimSpace(server.Status),
		ImageID:        firstNonEmpty(extractResourceID(server.Image), opts.ImageRef),
		FlavorID:       firstNonEmpty(extractResourceID(server.Flavor), opts.FlavorRef),
		KeyName:        keyName,
		SecurityGroups: securityGroups,
		FixedIP:        fixedIP,
		FloatingIP:     floatingIP,
	}
}

func extractServerIPs(server *Server) (string, string) {
	var fixedIP string
	var floatingIP string

	for _, rawAddresses := range server.Addresses {
		addresses := decodeServerAddresses(rawAddresses)
		for _, address := range addresses {
			ip := strings.TrimSpace(address.Address)
			if ip == "" {
				continue
			}

			switch strings.TrimSpace(address.Type) {
			case "floating":
				if floatingIP == "" {
					floatingIP = ip
				}
			case "fixed":
				if fixedIP == "" {
					fixedIP = ip
				}
			}
		}
	}

	if floatingIP == "" {
		floatingIP = strings.TrimSpace(server.AccessIPv4)
	}

	return fixedIP, floatingIP
}

func decodeServerAddresses(rawAddresses any) []serverAddress {
	if rawAddresses == nil {
		return nil
	}

	payload, err := json.Marshal(rawAddresses)
	if err != nil {
		return nil
	}

	var addresses []serverAddress
	if err := json.Unmarshal(payload, &addresses); err != nil {
		return nil
	}

	return addresses
}

func extractResourceID(resource map[string]any) string {
	if resource == nil {
		return ""
	}

	if id, ok := resource["id"].(string); ok {
		return strings.TrimSpace(id)
	}

	return ""
}

func extractSecurityGroupNames(groups []map[string]any, fallback []string) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		name, _ := group["name"].(string)
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}

	if len(names) > 0 {
		return names
	}

	if len(fallback) == 0 {
		return nil
	}

	cloned := make([]string, len(fallback))
	copy(cloned, fallback)
	return cloned
}

func extractUsageStats(diag map[string]any) UsageStats {
	if diag == nil {
		return UsageStats{}
	}
	var usage UsageStats
	if cpu, ok := diag["cpu0_time"].(float64); ok {
		usage.CPUUsage = cpu
	}
	if mem, ok := diag["memory"].(float64); ok {
		usage.MemoryUsage = int(mem / 1024)
	}
	return usage
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}
