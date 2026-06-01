package main

import (
	"context"
	"fmt"

	"github.com/KHU-RETURN/rcp-server/ent"
	entinstance "github.com/KHU-RETURN/rcp-server/ent/instance"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

// VM is the gateway's view of an instance — only what's needed to render the
// menu and dial a SOCKS5 CONNECT.
type VM struct {
	OpenstackID string
	Name        string
	Status      string
}

type repo struct {
	c *ent.Client
}

func newRepo(c *ent.Client) *repo { return &repo{c: c} }

// ListInstancesByEmail returns the user's instances ordered by name.
func (r *repo) ListInstancesByEmail(ctx context.Context, email string) ([]VM, error) {
	rows, err := r.c.Instance.Query().
		Where(entinstance.HasOwnerWith(entuser.Email(email))).
		Order(entinstance.ByName()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list instances for %s: %w", email, err)
	}
	out := make([]VM, 0, len(rows))
	for _, row := range rows {
		out = append(out, VM{
			OpenstackID: row.OpenstackID,
			Name:        row.Name,
			Status:      row.Status,
		})
	}
	return out, nil
}
