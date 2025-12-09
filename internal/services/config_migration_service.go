package services

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	jsonparser "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/wailsapp/wails/v3/pkg/services/log"
)

const (
	// BackupFilePattern 备份文件名模式
	BackupFilePattern = "%s.backup.%s.json"
	// MaxConfigFileSize 最大配置文件大小限制为10MB
	MaxConfigFileSize = 10 * 1024 * 1024 // 10MB
)

// ConfigMigrator 是一个优雅的配置迁移器，支持自动字段检测功能
// 它能够自动识别配置文件中的字段变化并进行相应的迁移处理
type ConfigMigrator struct {
	logger     *log.LogService // 日志服务实例，用于记录迁移过程中的日志信息
	configDir  string          // 配置文件所在目录路径
	configName string          // 配置文件名称（不包含扩展名）
	configPath string          // 完整的配置文件路径
}

// MigrationResult 迁移操作结果
type MigrationResult struct {
	Migrated      bool     `json:"migrated"`      // 是否执行了迁移操作
	MissingFields []string `json:"missingFields"` // 缺失的字段列表
	BackupPath    string   `json:"backupPath"`    // 备份文件路径
	Description   string   `json:"description"`   // 迁移描述信息
}

// NewConfigMigrator 创建一个新的配置迁移器
// 参数说明:
//   - logger: 日志服务实例，如果为nil则会创建默认日志服务
//   - configDir: 配置文件目录路径
//   - configName: 配置文件名称
//   - configPath: 配置文件完整路径
//
// 返回值:
//   - *ConfigMigrator: 返回新的配置迁移器实例
func NewConfigMigrator(
	logger *log.LogService,
	configDir string,
	configName, configPath string,
) *ConfigMigrator {
	// 如果未提供日志服务，则创建默认日志服务
	if logger == nil {
		logger = log.New()
	}
	return &ConfigMigrator{
		logger:     logger,
		configDir:  configDir,
		configName: configName,
		configPath: configPath,
	}
}

// AutoMigrate 自动检测并迁移缺失的配置字段
// 该方法会比较当前配置与默认配置，找出缺失的字段并进行迁移处理
func (cm *ConfigMigrator) AutoMigrate(defaultConfig interface{}, currentConfig *koanf.Koanf) (*MigrationResult, error) {
	// 创建新的koanf实例用于加载默认配置
	defaultKoanf := koanf.New(".")
	// 加载默认配置结构体到koanf中
	if err := defaultKoanf.Load(structs.Provider(defaultConfig, "json"), nil); err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}

	// 检测缺失的字段
	missingFields := cm.detectMissingFields(currentConfig.All(), defaultKoanf.All())

	// 创建迁移结果对象
	result := &MigrationResult{
		MissingFields: missingFields,
		Migrated:      len(missingFields) > 0,
		Description:   fmt.Sprintf("找到 %d 个缺失的字段", len(missingFields)),
	}

	// 如果没有需要迁移的字段，则直接返回结果
	if !result.Migrated {
		return result, nil
	}

	// 在迁移前创建配置文件备份
	backupPath, err := cm.createBackup()
	if err != nil {
		return result, fmt.Errorf("备份创建失败: %w", err)
	}
	result.BackupPath = backupPath

	// 将默认配置中的缺失字段合并到当前配置中
	if err := cm.mergeDefaultFields(currentConfig, defaultKoanf, missingFields); err != nil {
		return result, fmt.Errorf("合并默认字段失败: %w", err)
	}

	// 保存更新后的配置
	if err := cm.saveConfig(currentConfig); err != nil {
		return result, fmt.Errorf("保存更新配置失败: %w", err)
	}

	// 迁移成功后清理备份文件
	if backupPath != "" {
		if err := os.Remove(backupPath); err != nil {
			cm.logger.Error("删除备份文件失败", "error", err)
		}
	}

	return result, nil
}

// detectMissingFields 检测当前配置中缺失的字段
// 该函数通过递归比较当前配置和默认配置，找出所有在当前配置中缺失的字段路径
//
// 参数:
//   - current: 当前配置映射，表示用户提供的配置
//   - defaultConfig: 默认配置映射，表示完整的参考配置
//
// 返回值:
//   - []string: 缺失字段的路径列表，每个路径表示一个在当前配置中缺失的字段
func (cm *ConfigMigrator) detectMissingFields(current, defaultConfig map[string]interface{}) []string {
	var missing []string
	cm.findMissing("", defaultConfig, current, &missing)
	return missing
}

// findMissing 递归查找当前配置中缺失的字段，将缺失的字段路径添加到missing切片中
// 参数:
//
//	prefix: 当前递归层级的字段前缀路径
//	defaultMap: 默认配置映射，用于对比标准结构
//	currentMap: 当前配置映射，需要检查缺失字段的配置
//	missing: 指向字符串切片的指针，用于收集缺失字段的完整路径
func (cm *ConfigMigrator) findMissing(prefix string, defaultMap, currentMap map[string]interface{}, missing *[]string) {
	for key, defaultVal := range defaultMap {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		currentVal, exists := currentMap[key]
		if !exists {
			// 字段完全缺失，将其添加到缺失列表中
			*missing = append(*missing, fullKey)
		} else if defaultNestedMap, ok := defaultVal.(map[string]interface{}); ok {
			if currentNestedMap, ok := currentVal.(map[string]interface{}); ok {
				// 两个值都是映射类型，递归深入比较子映射
				cm.findMissing(fullKey, defaultNestedMap, currentNestedMap, missing)
			}
			// 类型不匹配：用户配置了不同类型的值，不进行递归比较
		}
		// 对于非映射类型的默认值，字段已存在，保留用户的配置值
	}
}

// mergeDefaultFields 将默认配置中的缺失字段合并到当前配置中
//
// 参数:
//   - current: 当前配置对象，将被修改以包含缺失的默认字段
//   - defaultConfig: 包含默认值的配置对象
//   - missingFields: 需要从默认配置中合并的字段列表
//
// 返回值:
//   - error: 返回nil，表示操作成功
func (cm *ConfigMigrator) mergeDefaultFields(current, defaultConfig *koanf.Koanf, missingFields []string) error {
	actuallyMerged := 0

	// 遍历所有缺失的字段，从默认配置中获取并设置到当前配置
	for _, field := range missingFields {
		if defaultConfig.Exists(field) {
			if defaultValue := defaultConfig.Get(field); defaultValue != nil {
				// 总是设置字段，即使可能引起类型冲突
				// 这允许在升级过程中配置结构的演进
				current.Set(field, defaultValue)
				actuallyMerged++
			}
		}
	}

	// 如果实际合并了字段，则更新时间戳
	if actuallyMerged > 0 {
		current.Set("metadata.lastUpdated", time.Now().Format(time.RFC3339))
	}

	return nil
}

// createBackup 创建配置文件的备份
// 该函数会在执行配置迁移前创建当前配置文件的备份，以防迁移过程中出现错误
//
// 返回值:
//   - string: 创建的备份文件路径，如果配置文件不存在则返回空字符串
//   - error: 创建备份过程中遇到的错误，成功时返回nil
func (cm *ConfigMigrator) createBackup() (string, error) {
	// 检查配置文件是否存在，如果不存在则无需创建备份
	if _, err := os.Stat(cm.configPath); os.IsNotExist(err) {
		return "", nil
	}

	// 生成时间戳并构建备份文件路径
	timestamp := time.Now().Format("20060102150405")
	backupPath := filepath.Join(cm.configDir, fmt.Sprintf(BackupFilePattern, cm.configName, timestamp))

	// 读取当前配置文件内容
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return "", fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 将配置内容写入备份文件
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("创建备份文件失败: %w", err)
	}

	return backupPath, nil
}

// saveConfig 将配置保存到文件中
// 参数:
//   - config: 要保存的配置对象
//
// 返回值:
//   - error: 保存过程中发生的错误
func (cm *ConfigMigrator) saveConfig(config *koanf.Koanf) error {
	// 将配置序列化为JSON格式的字节流
	configBytes, err := config.Marshal(jsonparser.Parser())
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 检查配置文件大小是否超过限制
	if len(configBytes) > MaxConfigFileSize {
		return fmt.Errorf("config size (%d bytes) exceeds limit (%d bytes)", len(configBytes), MaxConfigFileSize)
	}

	// 原子写入配置文件，避免写入过程中出现中断导致文件损坏
	tempPath := cm.configPath + ".tmp"
	if err := os.WriteFile(tempPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}

	// 将临时文件重命名为目标配置文件路径
	if err := os.Rename(tempPath, cm.configPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp config: %w", err)
	}

	return nil
}
