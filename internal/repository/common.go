package repository

import (
	"context"
	"errors"

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
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// isUniqueErr 判断错误是否为唯一索引冲突
func isUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "UNIQUE constraint failed")
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

// contains 判断字符串是否包含子串
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && stringIndex(s, substr) >= 0)
}

// stringIndex 返回子串首次出现的位置
func stringIndex(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
