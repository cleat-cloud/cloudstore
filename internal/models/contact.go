package models

import "time"

type Contact struct {
	ID        int64
	Name      string
	Email     string
	CreatedAt time.Time
}
