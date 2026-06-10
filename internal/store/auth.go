package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a lookup yields no row.
var ErrNotFound = errors.New("store: not found")

func (s *Store) CountAdminUsers(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&AdminUser{}).Count(&n).Error
	return n, err
}

func (s *Store) CreateAdminUser(ctx context.Context, login, passwordHash string) (AdminUser, error) {
	user := AdminUser{TenantID: DefaultTenantID, Login: login, PasswordHash: passwordHash}
	if err := s.db.WithContext(ctx).Create(&user).Error; err != nil {
		return AdminUser{}, err
	}
	return user, nil
}

func (s *Store) GetAdminUserByLogin(ctx context.Context, login string) (AdminUser, error) {
	var user AdminUser
	err := s.db.WithContext(ctx).Where("login = ?", login).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AdminUser{}, ErrNotFound
	}
	return user, err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, userID uint, passwordHash string) error {
	return s.db.WithContext(ctx).Model(&AdminUser{}).
		Where("id = ?", userID).
		Update("password_hash", passwordHash).Error
}

func (s *Store) CreateSession(ctx context.Context, token string, userID uint, expiresAt time.Time) error {
	return s.db.WithContext(ctx).Create(&Session{Token: token, UserID: userID, ExpiresAt: expiresAt}).Error
}

func (s *Store) GetValidSession(ctx context.Context, token string, now time.Time) (Session, error) {
	var sess Session
	err := s.db.WithContext(ctx).
		Where("token = ? AND expires_at > ?", token, now).
		First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Session{}, ErrNotFound
	}
	return sess, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	return s.db.WithContext(ctx).Where("token = ?", token).Delete(&Session{}).Error
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&Session{}).Error
}
