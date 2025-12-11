package services

import (
	"sync"
	"time"
	"voidraft/internal/common/translator"

	"github.com/wailsapp/wails/v3/pkg/services/log"
)

// TranslationService 翻译服务
// 提供翻译功能的核心服务结构体，管理多种翻译器实例并提供翻译服务
type TranslationService struct {
	logger         *log.LogService                                     // 日志服务实例，用于记录翻译过程中的日志信息
	factory        *translator.TranslatorFactory                       // 翻译器工厂，用于创建不同类型的翻译器实例
	defaultTimeout time.Duration                                       // 默认超时时间，用于控制翻译请求的最大等待时间
	translators    map[translator.TranslatorType]translator.Translator // 翻译器映射表，存储已创建的翻译器实例
	mutex          sync.RWMutex                                        // 读写锁，保证并发访问翻译器映射表的安全性
}

// NewTranslationService 创建翻译服务实例
// NewTranslationService 创建一个新的翻译服务实例
//
// 参数:
//
//	logger - 日志服务实例，用于记录翻译过程中的日志信息
//
// 返回值:
//
//	*TranslationService - 初始化完成的翻译服务实例
func NewTranslationService(logger *log.LogService) *TranslationService {
	// 初始化翻译服务的基本配置
	service := &TranslationService{
		logger:         logger,
		factory:        translator.NewTranslatorFactory(),
		defaultTimeout: 10 * time.Second,
		translators:    make(map[translator.TranslatorType]translator.Translator),
	}
	return service
}

// getTranslator 获取指定类型的翻译器，如不存在则创建
// getTranslator 根据翻译器类型获取对应的翻译器实例
// 如果该类型的翻译器已存在则直接返回，否则创建新的实例并缓存
//
// 参数:
//
//	translatorType - 翻译器类型枚举值
//
// 返回值:
//
//	translator.Translator - 翻译器接口实例
//	error - 获取失败时返回的错误信息
func (s *TranslationService) getTranslator(translatorType translator.TranslatorType) (translator.Translator, error) {
	s.mutex.RLock()
	trans, exists := s.translators[translatorType]
	s.mutex.RUnlock()

	if exists {
		return trans, nil
	}

	// 创建新的翻译器实例
	trans, err := s.factory.Create(translatorType)
	if err != nil {
		return nil, err
	}

	trans.SetTimeout(s.defaultTimeout)

	s.mutex.Lock()
	s.translators[translatorType] = trans
	s.mutex.Unlock()

	return trans, nil
}

// TranslateWith 使用指定翻译器进行翻译
// @param {string} text - 待翻译文本
// @param {string} from - 源语言代码 (如 "en", "zh", "auto")
// @param {string} to - 目标语言代码 (如 "en", "zh")
// @param {string} translatorType - 翻译器类型 ("google", "bing", "youdao", "deepl")
// @returns {string} 翻译后的文本
// @returns {error} 可能的错误
func (s *TranslationService) TranslateWith(text string, from string, to string, translatorType string) (string, error) {
	// 空文本直接返回
	if text == "" {
		return "", nil
	}
	if translatorType == "" {
		translatorType = string(translator.BingTranslatorType)
	}

	// 转换为翻译器类型
	transType := translator.TranslatorType(translatorType)

	// 获取指定翻译器
	trans, err := s.getTranslator(transType)
	if err != nil {
		return "", err
	}

	// 创建翻译参数
	params := translator.TranslationParams{
		From:    from,
		To:      to,
		Timeout: s.defaultTimeout,
	}

	// 执行翻译
	return trans.TranslateWithParams(text, params)
}

// GetTranslators 获取所有可用翻译器类型
// @returns {[]string} 翻译器类型列表
func (s *TranslationService) GetTranslators() []string {
	return []string{
		string(translator.BingTranslatorType),
		string(translator.GoogleTranslatorType),
		string(translator.YoudaoTranslatorType),
		string(translator.DeeplTranslatorType),
		string(translator.TartuNLPTranslatorType),
	}
}

// GetTranslatorLanguages 获取翻译器的语言列表
// @param {string} translatorType - 翻译器类型 ("google", "bing", "youdao", "deepl")
// @returns {map[string]string} 语言代码到名称的映射
// @returns {error} 可能的错误
func (s *TranslationService) GetTranslatorLanguages(translatorType translator.TranslatorType) (map[string]translator.LanguageInfo, error) {
	translator, err := s.getTranslator(translatorType)
	if err != nil {
		return nil, err
	}
	// 获取语言列表
	languages := translator.GetSupportedLanguages()
	return languages, nil
}

// IsLanguageSupported 检查指定的语言代码是否受支持
// IsLanguageSupported 检查指定翻译器是否支持给定的语言代码
// translatorType: 翻译器类型
// languageCode: 要检查的语言代码
// 返回值: 如果语言被支持则返回true，否则返回false
func (s *TranslationService) IsLanguageSupported(translatorType translator.TranslatorType, languageCode string) bool {
	// 获取指定类型的翻译器实例
	translator, err := s.getTranslator(translatorType)
	if err != nil {
		return false
	}
	// 调用翻译器的IsLanguageSupported方法进行语言支持性检查
	return translator.IsLanguageSupported(languageCode)
}
