package apps

import "github.com/google/uuid"

type App struct {
	ID         uuid.UUID
	InstanceID string
	Subdomain  string
	Host       string
}

type RegisterAppRequest struct {
	Subdomain string `json:"subdomain" binding:"required"`
}

type AppResponse struct {
	ID         uuid.UUID `json:"id"`
	InstanceID string    `json:"instance_id"`
	Subdomain  string    `json:"subdomain"`
	Host       string    `json:"host"`
}

func appResponse(app *App) AppResponse {
	return AppResponse{
		ID:         app.ID,
		InstanceID: app.InstanceID,
		Subdomain:  app.Subdomain,
		Host:       app.Host,
	}
}
