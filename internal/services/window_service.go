package services

import (
	"context"
	"fmt"
	"strconv"
	"voidraft/internal/common/constant"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/log"
)

// WindowService 窗口管理服务
// 提供窗口相关的管理功能，包括窗口操作、文档关联和吸附功能
type WindowService struct {
	logger          *log.LogService
	documentService *DocumentService
	// 吸附服务引用
	windowSnapService *WindowSnapService
}

// NewWindowService 创建新的窗口服务实例
// @param logger 日志服务实例，如果为nil则会创建默认日志服务
// @param documentService 文档服务实例，用于处理文档相关操作
// @param windowSnapService 窗口快照服务实例，用于窗口状态管理
// @return *WindowService 返回初始化完成的窗口服务实例
func NewWindowService(logger *log.LogService, documentService *DocumentService, windowSnapService *WindowSnapService) *WindowService {
	// 如果未提供日志服务，则使用默认日志服务
	if logger == nil {
		logger = log.New()
	}

	return &WindowService{
		logger:            logger,
		documentService:   documentService,
		windowSnapService: windowSnapService,
	}
}

// ServiceStartup 服务启动时初始化
// @param ctx 上下文对象，用于控制服务启动过程的生命周期
// @param options 服务启动选项配置
// @return error 服务启动过程中可能产生的错误信息
func (ws *WindowService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	// 更新主窗口缓存数据
	ws.windowSnapService.UpdateMainWindowCache()
	return nil
}

// OpenDocumentWindow 为指定文档ID打开新窗口
//
// 参数:
//
//	documentID: 要打开的文档唯一标识符
//
// 返回值:
//
//	error: 打开窗口过程中发生的错误，如果成功则返回nil
func (ws *WindowService) OpenDocumentWindow(documentID int64) error {
	app := application.Get()
	windowName := strconv.FormatInt(documentID, 10)

	// 检查窗口是否已经存在
	if existingWindow, exists := app.Window.GetByName(windowName); exists {
		// 窗口已存在，显示并聚焦
		existingWindow.Show()
		existingWindow.Restore()
		existingWindow.Focus()
		return nil
	}

	// 获取文档信息
	doc, err := ws.documentService.GetDocumentByID(documentID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found: %d", documentID)
	}

	// 创建新窗口
	newWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       windowName,
		Title:                      fmt.Sprintf("voidraft - %s", doc.Title),
		Width:                      constant.VOIDRAFT_WINDOW_WIDTH,
		Height:                     constant.VOIDRAFT_WINDOW_HEIGHT,
		Hidden:                     false,
		Frameless:                  true,
		DevToolsEnabled:            false,
		DefaultContextMenuDisabled: false,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		Windows: application.WindowsWindow{
			Theme: application.SystemDefault,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              fmt.Sprintf("/?documentId=%d", documentID),
	})

	// 注册窗口事件
	ws.registerWindowEvents(newWindow, documentID)

	// 向吸附服务注册新窗口
	if ws.windowSnapService != nil {
		ws.windowSnapService.RegisterWindow(documentID, newWindow)
	}

	// 最后才移动窗口到中心
	newWindow.Center()

	return nil
}

// registerWindowEvents 注册窗口事件
// 该函数为指定的webview窗口注册相关的事件处理函数
// 参数:
//
//	window: Webview窗口实例，用于注册事件钩子
//	documentID: 文档标识符，用于标识具体的文档窗口
func (ws *WindowService) registerWindowEvents(window *application.WebviewWindow, documentID int64) {
	// 注册窗口关闭事件处理器，当窗口即将关闭时触发相应的业务逻辑
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		ws.onWindowClosing(documentID)
	})
}

// onWindowClosing 处理窗口关闭事件
// documentID: 窗口对应的文档ID
func (ws *WindowService) onWindowClosing(documentID int64) {
	// 从吸附服务中取消注册
	if ws.windowSnapService != nil {
		ws.windowSnapService.UnregisterWindow(documentID)
	}
}

// GetOpenWindows 获取所有打开的文档窗口
//
// 返回值:
//   - []application.Window: 包含所有打开窗口的切片
func (ws *WindowService) GetOpenWindows() []application.Window {
	// 获取应用程序实例
	app := application.Get()
	// 返回所有窗口列表
	return app.Window.GetAll()
}

// IsDocumentWindowOpen 检查指定文档的窗口是否已打开
// 参数:
//
//	documentID - 文档的唯一标识符
//
// 返回值:
//
//	bool - 如果文档窗口已打开则返回true，否则返回false
func (ws *WindowService) IsDocumentWindowOpen(documentID int64) bool {
	// 获取应用程序实例
	app := application.Get()
	// 将文档ID转换为窗口名称
	windowName := strconv.FormatInt(documentID, 10)
	// 根据窗口名称查找窗口是否存在
	_, exists := app.Window.GetByName(windowName)
	return exists
}

// ServiceShutdown 实现服务关闭接口
// 该函数负责在服务关闭时进行清理工作，主要包括从吸附服务中取消注册所有打开的窗口
// 返回值：error类型，表示关闭过程中可能发生的错误，当前实现始终返回nil
func (ws *WindowService) ServiceShutdown() error {
	// 从吸附服务中取消注册所有窗口
	if ws.windowSnapService != nil {
		windows := ws.GetOpenWindows()
		for _, window := range windows {
			if documentID, err := strconv.ParseInt(window.Name(), 10, 64); err == nil {
				ws.windowSnapService.UnregisterWindow(documentID)
			}
		}
	}
	return nil
}
