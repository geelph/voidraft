package main

import (
	"embed"
	_ "embed"
	"log/slog"
	"time"
	"voidraft/internal/common/constant"
	"voidraft/internal/services"
	"voidraft/internal/systray"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

// main 函数是应用程序的入口点。它初始化应用程序、创建窗口，并启动一个协程，
// 每秒发送一次基于时间的事件。随后运行应用程序并记录可能发生的错误。
func main() {
	// 创建服务管理器实例，用于管理应用程序的各种服务
	serviceManager := services.NewServiceManager()

	// 声明Webview窗口变量，用于创建和管理应用程序的主窗口界面
	var window *application.WebviewWindow

	// 定义32字节的加密密钥数组，用于数据加密和解密操作
	// 该密钥采用固定的字节序列，按特定规律排列以确保加密安全性
	var encryptionKey = [32]byte{
		0x1e, 0x1f, 0x1c, 0x1d, 0x1a, 0x1b, 0x18, 0x19,
		0x16, 0x17, 0x14, 0x15, 0x12, 0x13, 0x10, 0x11,
		0x0e, 0x0f, 0x0c, 0x0d, 0x0a, 0x0b, 0x08, 0x09,
		0x06, 0x07, 0x04, 0x05, 0x02, 0x03, 0x00, 0x01,
	}

	// 创建一个新的应用程序实例
	// 该函数初始化应用程序的核心配置，包括基本信息、服务管理、资源处理、日志级别、Mac特定选项和单实例控制
	app := application.New(application.Options{
		// 应用程序名称，用于标识应用
		Name: constant.VOIDRAFT_APP_NAME,
		// 应用程序描述信息
		Description: constant.VOIDRAFT_APP_DESCRIPTION,
		// 注册应用程序所需的服务组件
		Services: serviceManager.GetServices(),
		// 资源文件配置选项
		Assets: application.AssetOptions{
			// 设置资源文件处理器，使用嵌入的assets文件系统
			Handler: application.AssetFileServerFS(assets),
		},
		// 设置日志级别为调试级别
		LogLevel: slog.LevelDebug,
		// Mac平台特定的配置选项
		Mac: application.MacOptions{
			// 当最后一个窗口关闭后应用程序应该终止运行
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		// 单实例运行配置，防止应用重复启动
		SingleInstance: &application.SingleInstanceOptions{
			// 使用应用名称作为唯一标识符
			UniqueID: constant.VOIDRAFT_APP_NAME,
			// 设置加密密钥用于实例间通信加密
			EncryptionKey: encryptionKey,
			// 当第二个实例启动时的回调处理函数
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// 如果主窗口存在，则显示并聚焦该窗口
				if window != nil {
					window.Show()
					window.Restore()
					window.Focus()
				}
			},
			// 附加数据，记录启动时间信息
			AdditionalData: map[string]string{
				"launchtime": time.Now().Local().String(),
			},
		},
	})

	// 创建主窗口并进行配置
	// 该函数创建一个带有特定配置的webview窗口，包括窗口大小、标题、样式等属性
	// Mac平台下设置了透明效果和隐藏标题栏，Windows平台使用系统默认主题
	// 窗口创建后会自动居中显示，并将窗口对象赋值给全局变量window
	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
		// 设置窗口名称，用于内部标识
		Name: constant.VOIDRAFT_MAIN_WINDOW_NAME,
		// 设置窗口标题，显示在窗口顶部
		Title: constant.VOIDRAFT_APP_NAME,
		// 设置窗口宽度为700像素
		Width: constant.VOIDRAFT_WINDOW_WIDTH,
		// 设置窗口高度为800像素
		Height: constant.VOIDRAFT_WINDOW_HEIGHT,
		// 窗口启动时是否隐藏，false表示不隐藏
		Hidden: false,
		// 是否启用无边框窗口模式，true表示启用
		Frameless: true,
		// 是否启用开发者工具，false表示禁用
		DevToolsEnabled: false,
		// 是否禁用默认上下文菜单，false表示不禁用
		DefaultContextMenuDisabled: false,
		// macOS平台特定配置
		Mac: application.MacWindow{
			// 设置无形标题栏的高度为50像素
			InvisibleTitleBarHeight: 50,
			// 设置窗口背景效果为半透明
			Backdrop: application.MacBackdropTranslucent,
			// 设置标题栏样式为隐藏内嵌式
			TitleBar: application.MacTitleBarHiddenInset,
		},
		// Windows平台特定配置
		Windows: application.WindowsWindow{
			// 设置窗口主题为系统默认主题
			Theme: application.SystemDefault,
		},
		// 设置窗口背景颜色为深蓝色RGB(27, 38, 54)
		BackgroundColour: application.NewRGB(27, 38, 54),
		// 设置窗口加载的初始URL路径为根路径
		URL: "/",
	})

	// 将窗口居中显示
	mainWindow.Center()

	// 将创建的主窗口赋值给全局window变量
	window = mainWindow

	// 获取系统托盘服务实例
	// 从服务管理器中获取托盘服务，用于管理系统托盘图标和相关操作
	trayService := serviceManager.GetTrayService()

	// 初始化并设置系统托盘功能
	systray.SetupSystemTray(mainWindow, assets, trayService)

	// 启动并运行整个应用程序。此调用会阻塞直到应用程序退出。
	err := app.Run()

	// 若运行过程中发生错误，则输出 panic 日志并终止程序执行。
	if err != nil {
		panic(err)
	}
}
