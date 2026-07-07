package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type computeClient interface {
	FetchFlavors() ([]Flavor, error)
	GetComputeQuota(projectID string) (*QuotaDetailSet, error)
	CreateServer(opts CreateServerOpts) (*Server, error)
	UpdateServerName(id string, name string) (*Server, error)
	PauseServer(id string) error
	UnpauseServer(id string) error
	DeleteServer(id string) error
	FetchInstances() ([]Server, error)
	FetchInstance(id string) (*Server, error)
	FetchDiagnostics(id string) (map[string]any, error)
}

type instanceRepo interface {
	SaveInstance(ctx context.Context, ownerID uuid.UUID, inst *Instance) error
	DeleteByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) error
	UpdateInstanceMetadata(ctx context.Context, ownerID uuid.UUID, openstackID string, update UpdateInstanceRequest) error
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Instance, error)
	FindByOpenstackID(ctx context.Context, ownerID uuid.UUID, openstackID string) (*Instance, error)
}

type Service struct {
	client           computeClient
	repo             instanceRepo
	projectID        string
	defaultNetworkID string

	flavorMu     sync.Mutex
	flavorCache  []Flavor
	flavorExpire time.Time
}

// flavorCacheTTL — flavor 목록은 거의 정적이라 요청마다 OpenStack에 묻지 않는다.
const flavorCacheTTL = 5 * time.Minute

var (
	ErrCreateInstanceNameRequired   = errors.New("name is required")
	ErrCreateInstanceImageRequired  = errors.New("image_id is required")
	ErrCreateInstanceFlavorRequired = errors.New("flavor_id is required")
	ErrInstanceNotFound             = errors.New("instance not found")
	ErrInstanceOperationFailed      = errors.New("instance operation failed")
)

func NewService(client computeClient, repo instanceRepo, projectID, defaultNetworkID string) *Service {
	return &Service{
		client:           client,
		repo:             repo,
		projectID:        projectID,
		defaultNetworkID: strings.TrimSpace(defaultNetworkID),
	}
}

// fetchFlavors는 flavor 목록을 TTL 캐시를 거쳐 반환한다.
func (s *Service) fetchFlavors() ([]Flavor, error) {
	s.flavorMu.Lock()
	defer s.flavorMu.Unlock()
	if time.Now().Before(s.flavorExpire) {
		return s.flavorCache, nil
	}
	raw, err := s.client.FetchFlavors()
	if err != nil {
		return nil, err
	}
	s.flavorCache = raw
	s.flavorExpire = time.Now().Add(flavorCacheTTL)
	return raw, nil
}

func (s *Service) GetFlavors() ([]FlavorResponse, error) {
	rawFlavors, err := s.fetchFlavors()
	if err != nil {
		return nil, err
	}

	var res []FlavorResponse
	for _, f := range rawFlavors {
		res = append(res, FlavorResponse(f))
	}
	return res, nil
}

func (s *Service) GetAvailableFlavorsWithLimit() ([]AvailableFlavorResponse, error) {
	rawFlavors, err := s.fetchFlavors()
	if err != nil {
		return nil, err
	}

	quota, err := s.client.GetComputeQuota(s.projectID)
	if err != nil {
		return nil, fmt.Errorf("쿼터 조회 실패: %v", err)
	}

	// Reserved: 빌드 진행 중인 예약분도 이미 소진된 용량이다.
	remCores := quota.Cores.Limit - quota.Cores.InUse - quota.Cores.Reserved
	remRAM := quota.RAM.Limit - quota.RAM.InUse - quota.RAM.Reserved
	remInstances := quota.Instances.Limit - quota.Instances.InUse - quota.Instances.Reserved

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
	staleIDs := make([]string, 0)

	for _, inst := range dbInstances {
		srv, ok := serverMap[inst.OpenstackID]
		if !ok {
			staleIDs = append(staleIDs, inst.OpenstackID)
			continue
		}

		fixedIP, floatingIP := extractServerIPs(&srv)
		res = append(res, InstanceDetailResponse{
			ID:         inst.OpenstackID,
			Name:       inst.Name,
			Status:     srv.Status,
			Image:      inst.ImageID,
			Flavor:     flavorMap[inst.FlavorID],
			KeyName:    firstNonEmpty(inst.KeyName, srv.KeyName),
			Note:       inst.Note,
			FixedIP:    fixedIP,
			FloatingIP: floatingIP,
			App:        inst.App,
			Created:    srv.Created,
		})
	}
	if err := s.pruneStaleInstances(ctx, ownerID, staleIDs); err != nil {
		return nil, err
	}

	return res, nil
}

// pruneStaleInstances는 목록 스냅샷에 없던 DB 행을 정리한다.
// 생성 직후 레이스(목록 API 지연)로 살아있는 서버의 소유권 행을 지우면
// 해당 VM이 API에서 영영 보이지 않게 되므로, 개별 조회로 404를 확인한 뒤에만 삭제한다.
func (s *Service) pruneStaleInstances(ctx context.Context, ownerID uuid.UUID, staleIDs []string) error {
	for _, id := range staleIDs {
		if _, err := s.client.FetchInstance(id); !isServerNotFound(err) {
			continue
		}
		if err := s.repo.DeleteByOpenstackID(ctx, ownerID, id); err != nil {
			return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
		}
	}

	return nil
}

// GetInstanceDetail은 DB(소유권), OpenStack 상태, diagnostics를 병렬로 읽어 조합하여 반환합니다.
func (s *Service) GetInstanceDetail(ctx context.Context, ownerID uuid.UUID, id string) (*InstanceDetailResponse, error) {
	var (
		inst                 *Instance
		srv                  *Server
		diag                 map[string]any
		flavorMap            map[string]FlavorResponse
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
		KeyName:    firstNonEmpty(inst.KeyName, srv.KeyName),
		Note:       inst.Note,
		FixedIP:    fixedIP,
		FloatingIP: floatingIP,
		App:        inst.App,
		Usage:      extractUsageStats(diag),
		Created:    srv.Created,
	}, nil
}

func (s *Service) UpdateInstance(ctx context.Context, ownerID uuid.UUID, id string, req UpdateInstanceRequest) (*InstanceDetailResponse, error) {
	inst, err := s.repo.FindByOpenstackID(ctx, ownerID, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	if inst == nil {
		return nil, ErrInstanceNotFound
	}

	req = normalizeUpdateInstanceRequest(req)
	// PATCH 시맨틱: 빈 필드는 기존 값 유지.
	// ponytail: 빈 문자열로 필드를 비우는 건 불가 — 필요해지면 *string DTO로 전환.
	if req.Name == "" {
		req.Name = inst.Name
	}
	if req.KeyName == "" {
		req.KeyName = inst.KeyName
	}
	if req.Note == "" {
		req.Note = inst.Note
	}

	// OpenStack 먼저, DB 나중: 원격 실패 시 DB가 먼저 바뀌어 두 시스템이 어긋나는 것을 막는다.
	if req.Name != inst.Name {
		if _, err := s.client.UpdateServerName(id, req.Name); err != nil {
			log.Printf("CRITICAL: Failed to update OpenStack name for %s: %v", id, err)

			return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
		}
	}

	if err := s.repo.UpdateInstanceMetadata(ctx, ownerID, id, req); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}

	return s.GetInstanceDetail(ctx, ownerID, id)
}

func (s *Service) PauseInstance(ctx context.Context, ownerID uuid.UUID, id string) error {
	inst, err := s.repo.FindByOpenstackID(ctx, ownerID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	if inst == nil {
		return ErrInstanceNotFound
	}
	if err := s.client.PauseServer(id); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	return nil
}

func (s *Service) UnpauseInstance(ctx context.Context, ownerID uuid.UUID, id string) error {
	inst, err := s.repo.FindByOpenstackID(ctx, ownerID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	if inst == nil {
		return ErrInstanceNotFound
	}
	if err := s.client.UnpauseServer(id); err != nil {
		return fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}
	return nil
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
	if len(normalizedOpts.Networks) == 0 && s.defaultNetworkID != "" {
		normalizedOpts.Networks = []NetworkID{{UUID: s.defaultNetworkID}}
	}
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
		Status:      normalizeServerStatus(server.Status),
		ImageID:     firstNonEmpty(extractResourceID(server.Image), normalizedOpts.ImageRef),
		FlavorID:    firstNonEmpty(extractResourceID(server.Flavor), normalizedOpts.FlavorRef),
		KeyName:     normalizedOpts.KeyName,
		Note:        normalizedOpts.Note,
		Created:     server.Created,
	}

	if err := s.repo.SaveInstance(ctx, ownerID, inst); err != nil {
		// 소유권 기록 실패 시 방금 만든 서버를 회수해 고아 VM을 막는다.
		if delErr := s.client.DeleteServer(server.ID); delErr != nil {
			log.Printf("CRITICAL: orphaned server %s: DB save failed (%v) and cleanup failed (%v)", server.ID, err, delErr)
		}
		return nil, fmt.Errorf("%w: %v", ErrInstanceOperationFailed, err)
	}

	return buildCreateInstanceResponse(server, normalizedOpts), nil
}

func (s *Service) fetchFlavorMap() (map[string]FlavorResponse, error) {
	rawFlavors, err := s.fetchFlavors()
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
		Note:           req.Note,
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
	req.Note = strings.TrimSpace(req.Note)
	req.SecurityGroups = normalizeStringSlice(req.SecurityGroups)
	return req
}

func normalizeUpdateInstanceRequest(req UpdateInstanceRequest) UpdateInstanceRequest {
	return UpdateInstanceRequest{
		Name:    strings.TrimSpace(req.Name),
		KeyName: strings.TrimSpace(req.KeyName),
		Note:    strings.TrimSpace(req.Note),
	}
}

func normalizeCreateServerOpts(opts CreateServerOpts) CreateServerOpts {
	opts.Name = strings.TrimSpace(opts.Name)
	opts.ImageRef = strings.TrimSpace(opts.ImageRef)
	opts.FlavorRef = strings.TrimSpace(opts.FlavorRef)
	opts.KeyName = strings.TrimSpace(opts.KeyName)
	opts.Note = strings.TrimSpace(opts.Note)
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
	case opts.Name == "":
		return ErrCreateInstanceNameRequired
	case opts.ImageRef == "":
		return ErrCreateInstanceImageRequired
	case opts.FlavorRef == "":
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
		Status:         normalizeServerStatus(server.Status),
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
		usage.CPUUsage = cpu / 1e9
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

func normalizeServerStatus(status string) string {
	return firstNonEmpty(status, "BUILD")
}
