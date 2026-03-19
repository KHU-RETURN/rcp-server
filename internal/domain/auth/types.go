package auth

import "time"

type User struct {
	ID           int64
	Email        string
	Name         string
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}