package store

import (
	"context"
	"fmt"
	"strings"

	"reviews/internal/config"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type Store struct {
	db *gorm.DB
}

func Open(cfg config.DBConfig) (*Store, error) {
	var dialector gorm.Dialector
	switch strings.ToLower(cfg.Driver) {
	case "sqlite":
		dialector = sqlite.Open(cfg.DSN)
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported DB driver %q", cfg.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(
		&Product{},
		&ProductMarketplaceLink{},
		&Review{},
		&ReviewMedia{},
		&SyncState{},
		&SyncRun{},
	)
}
