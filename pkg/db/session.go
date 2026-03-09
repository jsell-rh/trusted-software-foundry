package db

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/jsell-rh/trusted-software-foundry/pkg/config"
)

type SessionFactory interface {
	Init(*config.DatabaseConfig)
	DirectDB() *sql.DB
	New(ctx context.Context) *gorm.DB
	CheckConnection() error
	Close() error
	ResetDB()
	NewListener(ctx context.Context, channel string, callback func(id string))
}
