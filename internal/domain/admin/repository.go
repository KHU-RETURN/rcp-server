package admin

import (
	"context"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/app"
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

func (r *Repository) Summary(ctx context.Context) (SummaryResponse, error) {
	users, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	instances, err := r.client.Instance.Query().Count(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	containers, err := r.client.Container.Query().Count(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	apps, err := r.client.App.Query().Count(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	keypairs, err := r.client.KeyPair.Query().Count(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}
	rows, err := r.client.Instance.Query().All(ctx)
	if err != nil {
		return SummaryResponse{}, err
	}

	statusCounts := make(map[string]int)
	for _, row := range rows {
		status := strings.ToUpper(strings.TrimSpace(row.Status))
		if status == "" {
			status = "UNKNOWN"
		}
		statusCounts[status]++
	}

	return SummaryResponse{
		Users:        users,
		Instances:    instances,
		Containers:   containers,
		Apps:         apps,
		Keypairs:     keypairs,
		StatusCounts: statusCounts,
	}, nil
}

func (r *Repository) Users(ctx context.Context, params PageParams) (PaginatedUsersResponse, error) {
	total, err := r.client.User.Query().Count(ctx)
	if err != nil {
		return PaginatedUsersResponse{}, err
	}
	rows, err := r.client.User.Query().
		WithInstances(func(q *ent.InstanceQuery) {
			q.WithApp()
		}).
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
		WithApp().
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

func (r *Repository) UserResources(ctx context.Context, rawID string) (UserResourcesResponse, error) {
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	row, err := r.client.User.Query().
		Where(user.ID(userID)).
		WithInstances(func(q *ent.InstanceQuery) {
			q.WithApp()
		}).
		WithContainers().
		WithKeypairs().
		Only(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	instanceRows, err := r.client.Instance.Query().
		Where(instance.HasOwnerWith(user.ID(userID))).
		WithOwner().
		WithApp().
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
	appRows, err := r.client.App.Query().
		Where(app.HasInstanceWith(instance.HasOwnerWith(user.ID(userID)))).
		WithInstance().
		Order(app.ByCreatedAt(sql.OrderDesc()), app.ByHost()).
		All(ctx)
	if err != nil {
		return UserResourcesResponse{}, err
	}

	res := UserResourcesResponse{
		User:       userResponse(row),
		Instances:  make([]InstanceResponse, 0, len(instanceRows)),
		Containers: make([]ContainerResponse, 0, len(containerRows)),
		Keypairs:   make([]KeypairResponse, 0, len(keypairRows)),
		Apps:       make([]AppResponse, 0, len(appRows)),
	}
	for _, item := range instanceRows {
		res.Instances = append(res.Instances, instanceResponse(item))
	}
	for _, item := range containerRows {
		res.Containers = append(res.Containers, containerResponse(item))
	}
	for _, item := range keypairRows {
		res.Keypairs = append(res.Keypairs, keypairResponse(item))
	}
	for _, item := range appRows {
		res.Apps = append(res.Apps, appResponse(item))
	}
	return res, nil
}

func userResponse(row *ent.User) UserResponse {
	appCount := 0
	for _, inst := range row.Edges.Instances {
		if inst.Edges.App != nil {
			appCount++
		}
	}
	return UserResponse{
		ID:             row.ID.String(),
		Email:          row.Email,
		Name:           row.Name,
		Role:           auth.RoleForEmail(row.Email),
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		InstanceCount:  len(row.Edges.Instances),
		ContainerCount: len(row.Edges.Containers),
		AppCount:       appCount,
		KeypairCount:   len(row.Edges.Keypairs),
	}
}

func instanceResponse(row *ent.Instance) InstanceResponse {
	owner := row.Edges.Owner
	app := row.Edges.App
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
	if app != nil {
		item.AppHost = app.Host
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

func appResponse(row *ent.App) AppResponse {
	instanceRow := row.Edges.Instance
	res := AppResponse{
		ID:        row.ID.String(),
		Host:      row.Host,
		Status:    "active",
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	if instanceRow != nil {
		res.InstanceID = instanceRow.OpenstackID
		res.InstanceName = instanceRow.Name
	}
	return res
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
