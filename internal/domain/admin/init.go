package admin

import "github.com/KHU-RETURN/rcp-server/ent"

func Init(entClient *ent.Client) *Handler {
	repo := NewRepository(entClient)
	svc := NewService(repo)
	return NewHandler(svc)
}
