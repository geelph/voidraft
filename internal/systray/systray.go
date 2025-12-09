package systray

import (
	"embed"
	"runtime"
	"time"
	"voidraft/internal/events"
	"voidraft/internal/services"
	"voidraft/internal/version"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

// SetupSystemTray 设置系统托盘及其功能
// SetupSystemTray 初始化并配置系统托盘功能
// 参数:
//   - mainWindow: 主窗口对象，用于托盘事件交互
//   - assets: 嵌入的静态资源文件系统，用于读取托盘图标
//   - trayService: 托盘服务实例，处理托盘相关业务逻辑
func SetupSystemTray(mainWindow *application.WebviewWindow, assets embed.FS, trayService *services.TrayService) {
	// 获取应用程序的单例实例
	// 该函数返回全局唯一的应用程序实例，确保整个应用生命周期中只有一个实例存在
	// 返回值: 指向应用程序单例实例的指针
	app := application.Get()

	// 创建系统托盘
	systray := app.SystemTray.New()
	// 设置提示
	systray.SetTooltip("voidraft\nversion: " + version.Version)
	// 设置标签
	systray.SetLabel("voidraft")
	// 设置图标
	iconBytes, err := assets.ReadFile("frontend/dist/appicon.png")
	if err != nil {
		panic(err)
	}
	systray.SetIcon(iconBytes)
	systray.SetDarkModeIcon(iconBytes)

	// 针对macOS系统的特殊图标处理
	// 使用模板图标以适配浅色/深色模式切换
	if runtime.GOOS == "darwin" {
		systray.SetTemplateIcon(icons.SystrayMacTemplate)
	}

	// WindowDebounce 设置窗口防抖动延迟时间
	//
	// 该函数用于设置系统托盘窗口操作的防抖动延迟时间，
	// 防止在短时间内频繁触发窗口显示/隐藏等操作
	// 注意事项:
	//   - 该延迟时间会影响系统托盘菜单的响应速度
	//   - 过短的延迟可能导致防抖动效果不佳
	//   - 过长的延迟可能影响用户体验
	systray.WindowDebounce(300 * time.Millisecond)

	// 创建托盘菜单
	menu := app.NewMenu()

	// 注册托盘菜单事件
	events.RegisterTrayMenuEvents(app, menu, mainWindow)

	// 将托盘菜单设置为系统托盘
	systray.SetMenu(menu)

	// 注册托盘相关事件
	events.RegisterTrayEvents(systray, mainWindow, trayService)
}
