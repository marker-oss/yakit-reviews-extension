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
	if err := s.db.WithContext(ctx).AutoMigrate(
		&Tenant{},
		&Product{},
		&ProductMarketplaceLink{},
		&ReviewerIdentity{},
		&Review{},
		&ReviewMedia{},
		&SyncState{},
		&SyncRun{},
		&AdminUser{},
		&Session{},
		&ShowcaseRule{},
		&ShowcasePin{},
		&WidgetConfig{},
		&MarketplaceCredential{},
		&AppSetting{},
		&Question{},
		&DSRLog{},
	); err != nil {
		return err
	}
	if err := s.migrateSyncStatePK(ctx); err != nil {
		return fmt.Errorf("sync state pk migration: %w", err)
	}
	if err := s.migrateTenantBackfill(ctx); err != nil {
		return fmt.Errorf("tenant backfill: %w", err)
	}
	if _, err := s.ScrubPersonalData(ctx); err != nil {
		return fmt.Errorf("scrub personal data: %w", err)
	}
	return nil
}

// migrateTenantBackfill stamps tenant 1 onto pre-multitenancy rows that were
// created before tenant_id existed (or with NULL) and seeds the default tenant.
func (s *Store) migrateTenantBackfill(ctx context.Context) error {
	tables := []string{
		"reviews", "reviewer_identities", "products", "product_marketplace_links",
		"questions", "app_settings", "widget_configs", "showcase_rules",
		"showcase_pins", "marketplace_credentials", "admin_users", "sessions",
		"sync_runs", "dsr_logs",
	}
	for _, table := range tables {
		if err := s.db.WithContext(ctx).Exec(
			"UPDATE " + table + " SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0",
		).Error; err != nil {
			return err
		}
	}
	return s.EnsureDefaultTenant(ctx, "")
}

// migrateSyncStatePK rebuilds sync_states when the tenant column was added to
// an existing table whose primary key still covers only marketplace (SQLite
// cannot alter a PK in place). The rebuild is a copy: safe and idempotent.
func (s *Store) migrateSyncStatePK(ctx context.Context) error {
	var pkColumns []string
	if err := s.db.WithContext(ctx).Raw(
		"SELECT name FROM pragma_table_info('sync_states') WHERE pk > 0 ORDER BY pk",
	).Scan(&pkColumns).Error; err != nil {
		return err
	}
	if len(pkColumns) == 2 { // tenant_id + marketplace — already composite
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE sync_states SET tenant_id = 1 WHERE tenant_id IS NULL OR tenant_id = 0").Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TABLE sync_states_new (
			tenant_id integer NOT NULL,
			marketplace text NOT NULL,
			last_synced_at datetime,
			backfilled numeric NOT NULL DEFAULT false,
			PRIMARY KEY (tenant_id, marketplace)
		)`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO sync_states_new (tenant_id, marketplace, last_synced_at, backfilled)
			SELECT COALESCE(tenant_id, 1), marketplace, last_synced_at, backfilled FROM sync_states`).Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE sync_states").Error; err != nil {
			return err
		}
		return tx.Exec("ALTER TABLE sync_states_new RENAME TO sync_states").Error
	})
}
