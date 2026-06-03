package main

import (
	"context"
	"fmt"

	"github.com/KHU-RETURN/rcp-server/ent"
	entapp "github.com/KHU-RETURN/rcp-server/ent/app"
)

type AppMapping struct {
	Host        string
	OpenstackID string
}

type repo struct {
	c *ent.Client
}

func newRepo(c *ent.Client) *repo { return &repo{c: c} }

func (r *repo) FindByHost(ctx context.Context, host string) (*AppMapping, error) {
	row, err := r.c.App.Query().
		Where(entapp.Host(host)).
		WithInstance().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errNotFound
		}
		return nil, fmt.Errorf("find app host %q: %w", host, err)
	}
	inst, err := row.Edges.InstanceOrErr()
	if err != nil {
		return nil, fmt.Errorf("load app instance for host %q: %w", host, err)
	}
	return &AppMapping{
		Host:        row.Host,
		OpenstackID: inst.OpenstackID,
	}, nil
}
