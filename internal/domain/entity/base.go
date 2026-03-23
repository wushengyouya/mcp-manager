package entity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base 定义通用基础字段
type Base struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// BeforeCreate 自动填充主键
func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}

// JSONStringList 定义字符串数组 JSON 类型
type JSONStringList []string

// Value 实现数据库写入
func (j JSONStringList) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

// Scan 实现数据库读取
func (j *JSONStringList) Scan(value any) error {
	return scanJSON(value, j)
}

// JSONStringMap 定义字符串字典 JSON 类型
type JSONStringMap map[string]string

// Value 实现数据库写入
func (j JSONStringMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

// Scan 实现数据库读取
func (j *JSONStringMap) Scan(value any) error {
	return scanJSON(value, j)
}

// JSONMap 定义任意 JSON 对象
type JSONMap map[string]any

// Value 实现数据库写入
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

// Scan 实现数据库读取
func (j *JSONMap) Scan(value any) error {
	return scanJSON(value, j)
}

// scanJSON 将数据库字段统一解码到目标对象中
func scanJSON(value any, target any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, target)
	case string:
		return json.Unmarshal([]byte(v), target)
	default:
		return fmt.Errorf("不支持的 JSON 类型: %T", value)
	}
}
