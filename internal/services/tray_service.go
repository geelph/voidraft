package services

import (
	"voidraft/internal/common/helper"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/log"
)

// TrayService 系统托盘服务
type TrayService struct {
	logger        *log.LogService      // 日志服务实例，用于记录托盘相关日志
	configService *ConfigService       // 配置服务实例，用于获取托盘配置信息
	windowHelper  *helper.WindowHelper // 窗口助手实例，用于处理窗口显示/隐藏操作
}

// NewTrayService 创建新的系统托盘服务实例
// @param logger 日志服务实例，用于记录系统托盘相关的日志信息
// @param configService 配置服务实例，用于获取和管理配置信息
// @return *TrayService 返回初始化后的系统托盘服务实例
func NewTrayService(logger *log.LogService, configService *ConfigService) *TrayService {
	return &TrayService{
		logger:        logger,
		configService: configService,
		windowHelper:  helper.NewWindowHelper(),
	}
}

// ShouldMinimizeToTray 检查是否应该最小化到托盘
// 返回值: bool - true表示应该最小化到托盘，false表示不应该最小化到托盘
func (ts *TrayService) ShouldMinimizeToTray() bool {
	// 获取系统配置
	config, err := ts.configService.GetConfig()
	if err != nil {
		return true // 默认行为：隐藏到托盘
	}

	// 根据配置决定是否启用系统托盘
	return config.General.EnableSystemTray
}

// HandleWindowClose 处理窗口关闭事件
// 根据配置决定是将窗口最小化到托盘还是直接退出应用程序
func (ts *TrayService) HandleWindowClose() {
	// 判断是否应该最小化到托盘
	if ts.ShouldMinimizeToTray() {
		// 隐藏到托盘
		ts.windowHelper.HideMainWindow()
	} else {
		// 直接退出应用
		application.Get().Quit()
	}
}

// HandleWindowMinimize 处理窗口最小化事件
// 当窗口需要最小化时，根据配置决定是否隐藏到系统托盘
func (ts *TrayService) HandleWindowMinimize() {
	// 判断是否应该最小化到托盘
	if ts.ShouldMinimizeToTray() {
		// 隐藏到托盘
		ts.windowHelper.HideMainWindow()
	}
}

// ShowWindow 显示主窗口
// 该函数用于将主窗口置顶并获得焦点，使其在桌面上可见并可交互
func (ts *TrayService) ShowWindow() {
	// 调用窗口助手的方法来聚焦主窗口
	ts.windowHelper.FocusMainWindow()
}

// MinimizeButtonClicked 处理标题栏最小化按钮点击事件
//
// 该函数通过调用windowHelper的MinimiseMainWindow方法来实现主窗口的最小化操作。
func (ts *TrayService) MinimizeButtonClicked() {
	ts.windowHelper.MinimiseMainWindow()
}

// AutoShowHide 自动显示/隐藏主窗口
// 该函数通过调用windowHelper的AutoShowMainWindow方法来实现主窗口的自动显示功能
func (ts *TrayService) AutoShowHide() {
	ts.windowHelper.AutoShowMainWindow()
}
