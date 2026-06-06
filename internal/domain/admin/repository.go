package admin

import (
	"context"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/KHU-RETURN/rcp-server/ent"
	"github.com/KHU-RETURN/rcp-server/ent/instance"
	"github.com/KHU-RETURN/rcp-server/ent/user"
	"github.com/KHU-RETURN/rcp-server/internal/domain/auth"
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

func (r *Repository) Users(ctx context.Context, limit int) ([]UserResponse, error) {
	rows, err := r.client.User.Query().
		WithInstances(func(q *ent.InstanceQuery) {
			q.WithApp()
		}).
		WithContainers().
		WithKeypairs().
		Order(user.ByCreatedAt(sql.OrderDesc()), user.ByEmail()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]UserResponse, 0, len(rows))
	for _, row := range rows {
		appCount := 0
		for _, inst := range row.Edges.Instances {
			if inst.Edges.App != nil {
				appCount++
			}
		}
		result = append(result, UserResponse{
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
		})
	}
	return result, nil
}

func (r *Repository) Instances(ctx context.Context, limit int) ([]InstanceResponse, error) {
	rows, err := r.client.Instance.Query().
		WithOwner().
		WithApp().
		Order(instance.ByCreatedAt(sql.OrderDesc()), instance.ByOpenstackID()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]InstanceResponse, 0, len(rows))
	for _, row := range rows {
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
		result = append(result, item)
	}
	return result, nil
}
