package helper

import (
	"strconv"

	"voidraft/internal/common/constant"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowHelper 窗口辅助工具
type WindowHelper struct{}

// NewWindowHelper 创建窗口辅助工具实例
func NewWindowHelper() *WindowHelper {
	return &WindowHelper{}
}

// GetMainWindow 获取主窗口实例
// 返回值:
//   - application.Window: 主窗口对象
//   - bool: 是否成功获取到主窗口
func (wh *WindowHelper) GetMainWindow() (application.Window, bool) {
	// 获取应用程序实例
	app := application.Get()
	// 根据预定义的主窗口名称查找并返回主窗口
	return app.Window.GetByName(constant.VOIDRAFT_MAIN_WINDOW_NAME)
}

// MustGetMainWindow 获取主窗口实例
// 如果窗口不存在则返回 nil
//
// 返回值:
//
//	application.Window - 主窗口实例，如果获取失败则返回nil
//	bool - 表示是否成功获取到主窗口
func (wh *WindowHelper) MustGetMainWindow() application.Window {
	// 调用GetMainWindow 获取主窗口
	window, ok := wh.GetMainWindow()
	if !ok {
		return nil
	}
	return window
}

// ShowMainWindow 显示主窗口
//
// 该函数尝试获取主窗口实例并将其显示出来。如果成功获取到主窗口并显示，
// 则返回true；否则返回false。
//
// 返回值：
//
//	bool - 成功显示主窗口时返回true，否则返回false
func (wh *WindowHelper) ShowMainWindow() bool {
	// 获取主窗口实例，如果获取失败则返回nil
	if window := wh.MustGetMainWindow(); window != nil {
		window.Show()
		return true
	}
	return false
}

// HideMainWindow 隐藏主窗口
//
// 该函数通过获取主窗口实例并调用其Hide方法来隐藏窗口。
//
// 返回值:
//   - bool: 隐藏成功返回true，否则返回false
func (wh *WindowHelper) HideMainWindow() bool {
	// 获取主窗口实例，如果获取成功则隐藏窗口
	if window := wh.MustGetMainWindow(); window != nil {
		window.Hide()
		return true
	}
	return false
}

// MinimiseMainWindow 最小化主窗口
//
// 该函数尝试获取主窗口实例并将其最小化。如果成功获取到主窗口并执行最小化操作，
// 则返回true；否则返回false。
//
// 返回值:
//
//	bool - 操作是否成功执行。true表示成功最小化主窗口，false表示获取主窗口失败。
func (wh *WindowHelper) MinimiseMainWindow() bool {
	// 获取主窗口实例，如果获取成功则执行最小化操作
	if window := wh.MustGetMainWindow(); window != nil {
		window.Minimise()
		return true
	}
	return false
}

// FocusMainWindow 聚焦主窗口
//
// 该函数用于将应用程序的主窗口显示到前台并获得焦点。
//
// 返回值:
//
//	bool - 如果成功聚焦主窗口则返回true，否则返回false
func (wh *WindowHelper) FocusMainWindow() bool {
	if window := wh.MustGetMainWindow(); window != nil {
		// 显示并聚焦窗口
		window.Show()
		window.Restore()
		window.Focus()
		return true
	}
	return false
}

// AutoShowMainWindow 自动显示主窗口
// 根据主窗口的当前状态，如果窗口已经显示则聚焦到窗口，否则显示窗口
func (wh *WindowHelper) AutoShowMainWindow() {
	window := wh.MustGetMainWindow()

	// 窗口已显示，则聚焦窗口
	if window.IsVisible() {
		window.Focus()
	} else {
		window.Show()
	}
}

// GetDocumentWindow 根据文档ID获取窗口
// 参数:
//
//	documentID - 文档的唯一标识符
//
// 返回值:
//
//	application.Window - 找到的窗口对象
//	bool - 是否成功找到窗口
func (wh *WindowHelper) GetDocumentWindow(documentID int64) (application.Window, bool) {
	app := application.Get()
	// 将文档ID转换为窗口名称
	windowName := strconv.FormatInt(documentID, 10)
	return app.Window.GetByName(windowName)
}

// GetAllDocumentWindows 获取所有文档窗口
//
// 该函数遍历应用程序的所有窗口，过滤掉主窗口后返回剩余的文档窗口列表。
// 主窗口通过预定义的常量VOIDRAFT_MAIN_WINDOW_NAME进行识别和排除。
//
// 返回值:
//
//	[]application.Window - 包含所有文档窗口的切片，不包括主窗口
func (wh *WindowHelper) GetAllDocumentWindows() []application.Window {
	app := application.Get()
	// 获取所有窗口
	allWindows := app.Window.GetAll()

	// 遍历所有窗口, 排除主窗口
	var docWindows []application.Window
	for _, window := range allWindows {
		// 跳过主窗口
		if window.Name() != constant.VOIDRAFT_MAIN_WINDOW_NAME {
			docWindows = append(docWindows, window)
		}
	}
	return docWindows
}

// FocusDocumentWindow 聚焦指定文档的窗口
// 参数:
//
//	documentID - 需要聚焦的文档ID
//
// 返回值:
//
//	bool - 如果成功聚焦窗口则返回true，否则返回false
func (wh *WindowHelper) FocusDocumentWindow(documentID int64) bool {
	// 获取指定文档的窗口，如果存在则显示、恢复并聚焦该窗口
	if window, exists := wh.GetDocumentWindow(documentID); exists {
		window.Show()
		window.Restore()
		window.Focus()
		return true
	}
	return false
}

// CloseDocumentWindow 关闭指定文档的窗口
// 参数:
//
//	documentID - 要关闭窗口的文档ID
//
// 返回值:
//
//	bool - 关闭成功返回true，否则返回false
func (wh *WindowHelper) CloseDocumentWindow(documentID int64) bool {
	// 获取文档对应的窗口，如果存在则关闭该窗口
	if window, exists := wh.GetDocumentWindow(documentID); exists {
		window.Close()
		return true
	}
	return false
}
