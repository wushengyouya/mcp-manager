package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

var (
	// ErrNotFound 表示资源不存在
	ErrNotFound = errors.New("resource not found")
	// ErrAlreadyExists 表示资源已存在
	ErrAlreadyExists = errors.New("resource already exists")
)

// normalizeErr 将底层 ORM 错误归一化为仓储错误
func normalizeErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if isUniqueErr(err) {
		return ErrAlreadyExists
	}
	return err
}

// isUniqueErr 判断错误是否为唯一索引冲突
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	message := err.Error()
	if strings.Contains(message, "UNIQUE constraint failed") {
		return true
	}
	return strings.Contains(message, "duplicate key value violates unique constraint")
}

// exists 判断满足条件的记录是否存在
func exists(ctx context.Context, db *gorm.DB, model any, query string, args ...any) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(model).Where(query, args...).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// normalizePage 规范化分页参数
func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	return page, pageSize
}
