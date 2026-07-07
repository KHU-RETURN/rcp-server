package admin

import (
	"context"
	"sync"

	"entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/container"
	"github.com/KHU-RETURN/rcp-server/ent/instance"
	"github.com/KHU-RETURN/rcp-server/ent/keypair"
	"github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
	"github.com/google/uuid"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

// instanceStatusRow is a lightweight id/status projection used by the summary.
type instanceStatusRow struct {
	ID     string
	Status string
}

// summaryCounts holds raw resource counts plus instance id/status pairs so the
// service can overlay live OpenStack statuses before building the response.
type summaryCounts struct {
	Users      int
	Containers int
	Keypairs   int
	Instances  []instanceStatusRow
}

func (r *Repository) SummaryCounts(ctx context.Context) (summaryCounts, error) {
	var (
		counts summaryCounts
		rows   []*ent.Instance
		errs   [4]error
		wg     sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		counts.Users, errs[0] = r.client.User.Query().Count(ctx)
	}()
	go func() {
		defer wg.Done()
		counts.Containers, errs[1] = r.client.Container.Query().Count(ctx)
	}()
	go func() {
		defer wg.Done()
		counts.Keypairs, errs[2] = r.client.KeyPair.Query().Count(ctx)
	}()
	go func() {
		defer wg.Done()
		// Only id/status pairs are needed; skip fetching full rows.
		rows, errs[3] = r.client.Instance.Query().
			Select(instance.FieldOpenstackID, instance.FieldStatus).
			All(ctx)
	}()
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return summaryCounts{}, err
		}
	}

	counts.Instances = make([]instanceStatusRow, 0, len(rows))
	for _, row := range rows {
		counts.Instances = append(counts.Instances, instanceStatusRow{ID: row.OpenstackID, Status: row.Status})
	}
	return counts, nil
}

func (r *Repository) Users(ctx context.Context, params PageParams) (PaginatedUsersResponse, error) {
	total, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return PaginatedUsersResponse{}, err
	}
	rows, err := r.client.User.Query().
		WithInstances().
		WithContainers().
		WithKeypairs().
		Order(user.ByCreatedAt(sql.OrderDesc()), user.ByEmail()).
		Offset(pageOffset(params)).
		Limit(params.Limit).
		All(ctx)
	if err != nil {
		return PaginatedUsersResponse{}, err
	}

	result := make([]UserResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, userResponse(row))
	}
	return PaginatedUsersResponse{
		Items:      result,
		Pagination: paginationResponse(params, total),
	}, nil
}

func (r *Repository) Instances(ctx context.Context, params PageParams) (PaginatedInstancesResponse, error) {
	total, err := r.client.Instance.Query().Count(ctx)
	if err != nil {
		return PaginatedInstancesResponse{}, err
	}
	rows, err := r.client.Instance.Query().
		WithOwner().
		Order(instance.ByCreatedAt(sql.OrderDesc()), instance.ByOpenstackID()).
		Offset(pageOffset(params)).
		Limit(params.Limit).
		All(ctx)
	if err != nil {
		return PaginatedInstancesResponse{}, err
	}

	result := make([]InstanceResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, instanceResponse(row))
	}
	return PaginatedInstancesResponse{
		Items:      result,
		Pagination: paginationResponse(params, total),
	}, nil
}

func (r *Repository) Instance(ctx context.Context, id string) (InstanceResponse, error) {
	row, err := r.client.Instance.Query().
		Where(instance.OpenstackID(id)).
		WithOwner().
		Only(ctx)
	if err != nil {
		return InstanceResponse{}, err
	}
	return instanceResponse(row), nil
}

func (r *Repository) Containers(ctx context.Context, params PageParams) (PaginatedContainersResponse, error) {
	total, err := r.client.Container.Query().Count(ctx)
	if err != nil {
		return PaginatedContainersResponse{}, err
	}
	rows, err := r.client.Container.Query().
		WithOwner().
		Order(container.ByCreatedAt(sql.OrderDesc()), container.ByName()).
		Offset(pageOffset(params)).
		Limit(params.Limit).
		All(ctx)
	if err != nil {
		return PaginatedContainersResponse{}, err
	}

	result := make([]ContainerResponse, 0, len(rows))
	for _, row := range rows {
		result = append(result, containerResponse(row))
	}
	return PaginatedContainersResponse{
		Items:      result,
		Pagination: paginationResponse(params, total),
	}, nil
}

func (r *Repository) Container(ctx context.Context, rawID string) (ContainerResponse, error) {
	containerID, err := uuid.Parse(rawID)
	if err != nil {
		return ContainerResponse{}, err
	}
	row, err := r.client.Container.Query().
		Where(container.ID(containerID)).
		WithOwner().
		Only(ctx)
	if err != nil {
		return ContainerResponse{}, err
	}
	return containerResponse(row), nil
}

func (r *Repository) UserResources(ctx context.Context, rawID string) (UserResourcesResponse, error) {
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	row, err := r.client.User.Query().
		Where(user.ID(userID)).
		Only(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	instanceRows, err := r.client.Instance.Query().
		Where(instance.HasOwnerWith(user.ID(userID))).
		WithOwner().
		Order(instance.ByCreatedAt(sql.OrderDesc()), instance.ByOpenstackID()).
		All(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}
	containerRows, err := r.client.Container.Query().
		Where(container.HasOwnerWith(user.ID(userID))).
		WithOwner().
		Order(container.ByCreatedAt(sql.OrderDesc()), container.ByName()).
		All(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}
	keypairRows, err := r.client.KeyPair.Query().
		Where(keypair.HasOwnerWith(user.ID(userID))).
		WithInstances().
		Order(keypair.ByCreatedAt(sql.OrderDesc()), keypair.ByOpenstackName()).
		All(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	res := UserResourcesResponse{
		User:       userResponse(row),
		Instances:  make([]InstanceResponse, 0, len(instanceRows)),
		Containers: make([]ContainerResponse, 0, len(containerRows)),
		Keypairs:   make([]KeypairResponse, 0, len(keypairRows)),
	}
	// The user row is fetched without edges; derive the counts from the
	// already-fetched resource rows instead of loading everything twice.
	res.User.InstanceCount = len(instanceRows)
	res.User.ContainerCount = len(containerRows)
	res.User.KeypairCount = len(keypairRows)
	for _, item := range instanceRows {
		res.Instances = append(res.Instances, instanceResponse(item))
	}
	for _, item := range containerRows {
		res.Containers = append(res.Containers, containerResponse(item))
	}
	for _, item := range keypairRows {
		res.Keypairs = append(res.Keypairs, keypairResponse(item))
	}
	return res, nil
}

func userResponse(row *ent.User) UserResponse {
	return UserResponse{
		ID:             row.ID.String(),
		Email:          row.Email,
		Name:           row.Name,
		Role:           auth.RoleForEmail(row.Email),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		InstanceCount:  len(row.Edges.Instances),
		ContainerCount: len(row.Edges.Containers),
		KeypairCount:   len(row.Edges.Keypairs),
	}
}

func instanceResponse(row *ent.Instance) InstanceResponse {
	owner := row.Edges.Owner
	item := InstanceResponse{
		ID:        row.OpenstackID,
		Name:      row.Name,
		Status:    row.Status,
		FlavorID:  row.FlavorID,
		ImageID:   row.ImageID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if owner != nil {
		item.OwnerID = owner.ID.String()
		item.OwnerEmail = owner.Email
		item.OwnerName = owner.Name
	}
	return item
}

func containerResponse(row *ent.Container) ContainerResponse {
	owner := row.Edges.Owner
	item := ContainerResponse{
		ID:            row.ID.String(),
		Name:          row.Name,
		Status:        "ready",
		OpenstackName: row.OpenstackName.String(),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
	if owner != nil {
		item.OwnerID = owner.ID.String()
		item.OwnerEmail = owner.Email
		item.OwnerName = owner.Name
	}
	return item
}

func keypairResponse(row *ent.KeyPair) KeypairResponse {
	return KeypairResponse{
		ID:            row.ID.String(),
		Name:          row.OpenstackName,
		Status:        "registered",
		Fingerprint:   row.Fingerprint,
		SourceType:    row.SourceType.String(),
		InstanceCount: len(row.Edges.Instances),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func pageOffset(params PageParams) int {
	return (params.Page - 1) * params.Limit
}

func paginationResponse(params PageParams, total int) PaginationResponse {
	totalPages := 0
	if total > 0 {
		totalPages = (total + params.Limit - 1) / params.Limit
	}
	return PaginationResponse{
		Page:       params.Page,
		PerPage:    params.Limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
