package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ThemeType 主题类型枚举
type ThemeType string

const (
	ThemeTypeDark  ThemeType = "dark"
	ThemeTypeLight ThemeType = "light"
)

// ThemeColorConfig 使用与前端 ThemeColors 相同的结构，存储任意主题键值
type ThemeColorConfig map[string]interface{}

// Value 实现 driver.Valuer 接口，用于将 ThemeColorConfig 存储到数据库
func (tc ThemeColorConfig) Value() (driver.Value, error) {
	if tc == nil {
		return json.Marshal(map[string]interface{}{})
	}
	return json.Marshal(tc)
}

// Scan 实现 sql.Scanner 接口，用于从数据库读取 ThemeColorConfig
// 该方法将数据库中的值扫描并解析为 ThemeColorConfig 类型
//
// 参数:
//   - value: 从数据库中读取的原始值，可以是 []byte、string 或 nil
//
// 返回值:
//   - error: 如果类型转换或 JSON 解析失败则返回错误，否则返回 nil
func (tc *ThemeColorConfig) Scan(value interface{}) error {
	// 处理空值情况，将其初始化为空的 ThemeColorConfig
	if value == nil {
		*tc = ThemeColorConfig{}
		return nil
	}

	// 根据输入值的类型将其转换为字节切片
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into ThemeColorConfig", value)
	}

	// 将字节数据解析为 map[string]interface{} 结构
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		return err
	}

	// 将解析后的数据赋值给目标 ThemeColorConfig
	*tc = data
	return nil
}

// Theme 主题配置结构体，用于定义系统的主题样式
// 包含主题的基本信息、类型、颜色配置以及默认状态等属性
type Theme struct {
	ID        int              `db:"id" json:"id"`                // 主题唯一标识符
	Name      string           `db:"name" json:"name"`            // 主题名称
	Type      ThemeType        `db:"type" json:"type"`            // 主题类型
	Colors    ThemeColorConfig `db:"colors" json:"colors"`        // 主题颜色配置
	IsDefault bool             `db:"is_default" json:"isDefault"` // 是否为默认主题
	CreatedAt string           `db:"created_at" json:"createdAt"` // 创建时间
	UpdatedAt string           `db:"updated_at" json:"updatedAt"` // 更新时间
}
