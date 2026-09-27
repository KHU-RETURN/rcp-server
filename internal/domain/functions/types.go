package functions

import (
	"time"

	"github.com/google/uuid"
)

type Function struct {
	ID           uuid.UUID  `json:"id"`
	OwnerID      uuid.UUID  `json:"-"`
	Name         string     `json:"name"`
	Wasm         []byte     `json:"-"`
	Language     string     `json:"language"`
	DataMode     bool       `json:"data_mode"`
	Source       []byte     `json:"-"`
	KeyHash      []byte     `json:"-"`
	KeyEnabled   bool       `json:"key_enabled"`
	KeyCreatedAt *time.Time `json:"key_created_at,omitempty"`
	KeyExpiresAt *time.Time `json:"key_expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode uint32 `json:"exit_code"`
}

type Deployment struct {
	Language string
	Source   []byte
	Wasm     []byte
	DataMode bool
}
