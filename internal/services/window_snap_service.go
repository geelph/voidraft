package services

import (
	"math"
	"sync"
	"time"
	"voidraft/internal/common/helper"
	"voidraft/internal/models"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/log"
)

// 防抖和检测常量
const (
	// 移动事件防抖阈值：连续移动事件间隔小于此值时忽略
	debounceThreshold = 30 * time.Millisecond

	// 用户拖拽检测阈值：快速移动被认为是用户主动拖拽
	// 设置为稍大于防抖阈值，确保逻辑一致
	dragDetectionThreshold = 40 * time.Millisecond
)

// WindowSnapService 窗口吸附服务
// 提供窗口自动吸附功能，管理主窗口和子窗口的位置关系，实现窗口间的智能对齐和吸附效果
type WindowSnapService struct {
	logger        *log.LogService
	configService *ConfigService
	windowHelper  *helper.WindowHelper
	mu            sync.RWMutex

	// 吸附配置
	snapEnabled bool // 是否启用窗口吸附功能

	// 自适应阈值参数
	baseThresholdRatio float64 // 基础阈值比例
	minThreshold       int     // 最小阈值(像素)
	maxThreshold       int     // 最大阈值(像素)

	// 位置缓存
	lastMainWindowPos  models.WindowPosition // 缓存主窗口位置
	lastMainWindowSize [2]int                // 缓存主窗口尺寸 [width, height]

	// 管理的窗口
	managedWindows map[int64]*models.WindowInfo         // documentID -> WindowInfo
	windowRefs     map[int64]*application.WebviewWindow // documentID -> Window引用

	// 窗口尺寸缓存
	windowSizeCache map[int64][2]int // documentID -> [width, height]

	// 事件循环保护
	isUpdatingPosition map[int64]bool // documentID -> 是否正在更新位置

	// 事件监听器清理函数
	mainMoveUnhook    func()           // 主窗口移动监听清理函数
	windowMoveUnhooks map[int64]func() // documentID -> 子窗口移动监听清理函数

	// 配置观察者取消函数
	cancelObserver CancelFunc
}

// NewWindowSnapService 创建一个新的窗口吸附服务实例
// 参数:
//   - logger: 日志服务实例，用于记录窗口吸附相关的日志信息，如果传入nil则会创建默认日志服务
//   - configService: 配置服务实例，用于获取和监听窗口吸附相关配置
//
// 返回值:
//   - *WindowSnapService: 初始化完成的窗口吸附服务实例
func NewWindowSnapService(logger *log.LogService, configService *ConfigService) *WindowSnapService {
	if logger == nil {
		logger = log.New()
	}

	// 从配置获取窗口吸附设置
	config, err := configService.GetConfig()
	snapEnabled := true // 默认启用

	if err == nil {
		snapEnabled = config.General.EnableWindowSnap
	}

	wss := &WindowSnapService{
		logger:             logger,
		configService:      configService,
		windowHelper:       helper.NewWindowHelper(),
		snapEnabled:        snapEnabled,
		baseThresholdRatio: 0.025, // 2.5%的主窗口宽度作为基础阈值
		minThreshold:       8,     // 最小8像素（小屏幕保底）
		maxThreshold:       40,    // 最大40像素（大屏幕上限）
		managedWindows:     make(map[int64]*models.WindowInfo),
		windowRefs:         make(map[int64]*application.WebviewWindow),
		windowSizeCache:    make(map[int64][2]int),
		isUpdatingPosition: make(map[int64]bool),
		windowMoveUnhooks:  make(map[int64]func()),
	}

	// 注册窗口吸附配置监听
	wss.cancelObserver = configService.Watch("general.enableWindowSnap", wss.onWindowSnapConfigChange)

	return wss
}

// onWindowSnapConfigChange 处理窗口快照配置变更事件
// 当窗口快照功能的配置发生变化时，该函数会被调用
//
// 参数:
//
//	oldValue - 配置变更前的旧值
//	newValue - 配置变更后的新值
//
// 返回值: 无
func (wss *WindowSnapService) onWindowSnapConfigChange(oldValue, newValue interface{}) {
	// 解析新的配置值，判断窗口快照功能是否启用
	enabled := false
	if newValue != nil {
		if val, ok := newValue.(bool); ok {
			enabled = val
		}
	}

	// 调用配置变更处理函数，通知服务配置已更新
	_ = wss.OnWindowSnapConfigChanged(enabled)
}

// RegisterWindow 注册需要吸附管理的窗口
// RegisterWindow 注册一个窗口到窗口快照服务中，用于跟踪和管理窗口的位置变化
// documentID: 文档唯一标识符，用于关联窗口和文档
// window: 要注册的Webview窗口对象
func (wss *WindowSnapService) RegisterWindow(documentID int64, window *application.WebviewWindow) {
	wss.mu.Lock()
	defer wss.mu.Unlock()

	// 获取初始位置
	x, y := window.Position()

	// 创建窗口信息结构体，初始化窗口的基本状态和位置信息
	windowInfo := &models.WindowInfo{
		DocumentID: documentID,
		IsSnapped:  false,
		SnapOffset: models.SnapPosition{X: 0, Y: 0},
		SnapEdge:   models.SnapEdgeNone,
		LastPos:    models.WindowPosition{X: x, Y: y},
		MoveTime:   time.Now(),
	}

	// 将窗口信息存储到管理映射中
	wss.managedWindows[documentID] = windowInfo
	wss.windowRefs[documentID] = window

	// 初始化窗口尺寸缓存
	wss.updateWindowSizeCacheLocked(documentID, window)

	// 如果这是第一个注册的窗口，启动主窗口事件监听
	if len(wss.managedWindows) == 1 {
		wss.setupMainWindowEvents()
	}

	// 为窗口设置移动事件监听
	wss.setupWindowEvents(window, windowInfo)
}

// UnregisterWindow 取消注册指定文档ID的窗口，清理相关资源和事件监听
//
// 参数:
//
//	documentID - 需要取消注册的窗口对应的文档ID
//
// 该函数会执行以下操作：
// 1. 获取互斥锁以保证线程安全
// 2. 清理指定窗口的子窗口移动事件监听器
// 3. 从各个管理映射中删除该窗口的相关数据
// 4. 如果没有剩余的管理窗口，则清理主窗口事件监听
func (wss *WindowSnapService) UnregisterWindow(documentID int64) {
	wss.mu.Lock()
	defer wss.mu.Unlock()

	// 清理子窗口事件监听
	if unhook, exists := wss.windowMoveUnhooks[documentID]; exists {
		unhook()
		delete(wss.windowMoveUnhooks, documentID)
	}

	delete(wss.managedWindows, documentID)
	delete(wss.windowRefs, documentID)
	delete(wss.windowSizeCache, documentID)
	delete(wss.isUpdatingPosition, documentID)

	// 如果没有管理的窗口了，取消主窗口事件监听
	if len(wss.managedWindows) == 0 {
		wss.cleanupMainWindowEvents()
	}
}

// SetSnapEnabled 设置窗口吸附功能的启用状态
// 参数:
//
//	enabled: true表示启用窗口吸附功能，false表示禁用窗口吸附功能
func (wss *WindowSnapService) SetSnapEnabled(enabled bool) {
	wss.mu.Lock()
	defer wss.mu.Unlock()

	if wss.snapEnabled == enabled {
		return
	}

	wss.snapEnabled = enabled

	// 如果禁用吸附，解除所有吸附窗口
	if !enabled {
		for _, windowInfo := range wss.managedWindows {
			if windowInfo.IsSnapped {
				windowInfo.IsSnapped = false
				windowInfo.SnapEdge = models.SnapEdgeNone
			}
		}
	}
}

// calculateAdaptiveThreshold 计算自适应吸附阈值
// calculateAdaptiveThreshold 根据主窗口宽度动态计算吸附阈值
// 返回值: 计算得到的吸附阈值，用于判断窗口是否应该吸附到屏幕边缘
func (wss *WindowSnapService) calculateAdaptiveThreshold() int {
	// 基于主窗口宽度计算阈值
	mainWidth := wss.lastMainWindowSize[0]
	if mainWidth == 0 {
		return wss.minThreshold // 默认最小值
	}

	// 计算基础阈值：主窗口宽度的2.5%
	adaptiveThreshold := int(float64(mainWidth) * wss.baseThresholdRatio)

	// 限制在最小和最大值之间
	if adaptiveThreshold < wss.minThreshold {
		return wss.minThreshold
	}
	if adaptiveThreshold > wss.maxThreshold {
		return wss.maxThreshold
	}

	return adaptiveThreshold
}

// GetCurrentThreshold 获取当前自适应阈值
// GetCurrentThreshold 获取当前的阈值
// 该函数通过读取锁保护并发访问，并调用calculateAdaptiveThreshold方法计算自适应阈值
//
// 返回值:
//
//	int - 当前计算得到的自适应阈值
func (wss *WindowSnapService) GetCurrentThreshold() int {
	// 获取读锁以保证并发安全
	wss.mu.RLock()
	defer wss.mu.RUnlock()

	// 调用自适应阈值计算方法并返回结果
	return wss.calculateAdaptiveThreshold()
}

// OnWindowSnapConfigChanged 处理窗口吸附配置变更
// 当窗口吸附功能的配置发生改变时，该函数会被调用以更新服务的启用状态
//
// 参数:
//
//	enabled: 布尔值，表示窗口吸附功能是否应该启用
//
// 返回值:
//
//	error: 错误信息，当前实现始终返回nil
func (wss *WindowSnapService) OnWindowSnapConfigChanged(enabled bool) error {
	// 调用SetSnapEnabled方法更新窗口吸附功能的启用状态
	wss.SetSnapEnabled(enabled)
	// 返回nil表示操作成功
	return nil
}

// setupMainWindowEvents 设置主窗口事件监听
// 该函数用于注册主窗口的移动事件监听器，确保只设置一次监听器
func (wss *WindowSnapService) setupMainWindowEvents() {
	// 如果已经设置过，不重复设置
	if wss.mainMoveUnhook != nil {
		return
	}

	// 在锁外获取主窗口，避免死锁风险
	wss.mu.Unlock()
	mainWindow, ok := wss.windowHelper.GetMainWindow()
	wss.mu.Lock()

	if !ok {
		return
	}

	// 监听主窗口移动事件，当窗口移动时触发onMainWindowMoved回调
	wss.mainMoveUnhook = mainWindow.RegisterHook(events.Common.WindowDidMove, func(event *application.WindowEvent) {
		wss.onMainWindowMoved()
	})

}

// cleanupMainWindowEvents 清理主窗口事件监听器
// 该函数用于取消对主窗口移动事件的监听，避免内存泄漏和重复监听
func (wss *WindowSnapService) cleanupMainWindowEvents() {
	// 调用清理函数取消监听
	if wss.mainMoveUnhook != nil {
		wss.mainMoveUnhook()
		wss.mainMoveUnhook = nil
	}
}

// setupWindowEvents 为子窗口设置事件监听
// setupWindowEvents 为指定窗口设置事件监听器，主要用于监听子窗口的移动事件
// 参数:
//
//	window: 需要监听的Webview窗口实例
//	windowInfo: 包含窗口信息的结构体指针
func (wss *WindowSnapService) setupWindowEvents(window *application.WebviewWindow, windowInfo *models.WindowInfo) {
	// 监听子窗口移动事件，保存清理函数
	unhook := window.RegisterHook(events.Common.WindowDidMove, func(event *application.WindowEvent) {
		wss.onChildWindowMoved(window, windowInfo)
	})

	// 保存清理函数以便后续取消监听
	wss.windowMoveUnhooks[windowInfo.DocumentID] = unhook
}

// updateMainWindowCacheLocked 更新主窗口缓存信息
// 该函数在持有互斥锁的情况下调用，用于获取并缓存主窗口的位置和大小信息
// 为了避免在持有锁期间执行可能引起死锁的操作，函数会在获取窗口信息时临时释放锁
func (wss *WindowSnapService) updateMainWindowCacheLocked() {
	mainWindow := wss.windowHelper.MustGetMainWindow()
	if mainWindow == nil {
		return
	}

	// 在锁外获取窗口信息,避免死锁
	wss.mu.Unlock()
	x, y := mainWindow.Position()
	w, h := mainWindow.Size()
	wss.mu.Lock()

	wss.lastMainWindowPos = models.WindowPosition{X: x, Y: y}
	wss.lastMainWindowSize = [2]int{w, h}
}

// UpdateMainWindowCache 更新主窗口缓存
// 该函数会获取互斥锁，确保线程安全地更新主窗口缓存数据
//
// 参数:
//   - wss: WindowSnapService服务实例的指针
func (wss *WindowSnapService) UpdateMainWindowCache() {
	wss.mu.Lock()
	defer wss.mu.Unlock()
	wss.updateMainWindowCacheLocked()
}

// updateWindowSizeCacheLocked 更新窗口尺寸缓存
// updateWindowSizeCacheLocked 更新指定文档ID的窗口尺寸缓存
// 该函数在持有锁的情况下调用，会临时释放锁来安全地获取窗口尺寸，
// 然后重新获取锁并更新缓存
//
// 参数:
//
//	documentID - 需要更新缓存的文档唯一标识符
//	window - 窗口对象，用于获取当前尺寸信息
func (wss *WindowSnapService) updateWindowSizeCacheLocked(documentID int64, window *application.WebviewWindow) {
	// 在锁外获取窗口尺寸，避免死锁
	wss.mu.Unlock()
	w, h := window.Size()
	wss.mu.Lock()

	wss.windowSizeCache[documentID] = [2]int{w, h}
}

// getWindowSizeCached 获取缓存的窗口尺寸，如果不存在则实时获取并缓存
// getWindowSizeCached 获取指定文档窗口的尺寸，优先从缓存中获取，如果缓存不存在则实时获取并更新缓存
// 参数:
//   - documentID: 文档唯一标识符，用于索引窗口尺寸缓存
//   - window: Webview窗口对象，当缓存未命中时用于实时获取窗口尺寸
//
// 返回值:
//   - int: 窗口宽度
//   - int: 窗口高度
func (wss *WindowSnapService) getWindowSizeCached(documentID int64, window *application.WebviewWindow) (int, int) {
	// 先检查缓存
	if size, exists := wss.windowSizeCache[documentID]; exists {
		return size[0], size[1]
	}

	// 缓存不存在，实时获取并缓存
	wss.updateWindowSizeCacheLocked(documentID, window)

	if size, exists := wss.windowSizeCache[documentID]; exists {
		return size[0], size[1]
	}

	// 直接返回实时尺寸
	wss.mu.Unlock()
	w, h := window.Size()
	wss.mu.Lock()
	return w, h
}

// onMainWindowMoved 主窗口移动事件处理
// onMainWindowMoved 处理主窗口移动事件，更新已吸附窗口的位置
// 当主窗口移动时，该函数会重新计算并更新所有已吸附窗口的位置
// 以保持它们相对于主窗口的吸附关系
func (wss *WindowSnapService) onMainWindowMoved() {
	if !wss.snapEnabled {
		return
	}

	// 先在锁外获取主窗口的位置和尺寸
	mainWindow := wss.windowHelper.MustGetMainWindow()
	if mainWindow == nil {
		return
	}

	x, y := mainWindow.Position()
	w, h := mainWindow.Size()

	wss.mu.Lock()
	defer wss.mu.Unlock()

	// 更新主窗口位置和尺寸缓存
	wss.lastMainWindowPos = models.WindowPosition{X: x, Y: y}
	wss.lastMainWindowSize = [2]int{w, h}

	// 只更新已吸附窗口的位置，无需重新检测所有窗口
	for _, windowInfo := range wss.managedWindows {
		if windowInfo.IsSnapped {
			wss.updateSnappedWindowPosition(windowInfo)
		}
	}
}

// onChildWindowMoved 子窗口移动事件处理
// onChildWindowMoved 处理子窗口移动事件，实现窗口吸附功能
// window: 触发移动事件的Webview窗口实例
// windowInfo: 窗口信息结构体，包含窗口的状态和位置信息
func (wss *WindowSnapService) onChildWindowMoved(window *application.WebviewWindow, windowInfo *models.WindowInfo) {
	if !wss.snapEnabled {
		return
	}

	// 事件循环保护：如果正在更新位置，忽略此次事件
	wss.mu.Lock()
	if wss.isUpdatingPosition[windowInfo.DocumentID] {
		wss.mu.Unlock()
		return
	}
	wss.mu.Unlock()

	x, y := window.Position()
	currentPos := models.WindowPosition{X: x, Y: y}

	wss.mu.Lock()
	defer wss.mu.Unlock()

	// 检查是否真的移动了（避免无效触发）
	if currentPos.X == windowInfo.LastPos.X && currentPos.Y == windowInfo.LastPos.Y {
		return
	}

	// 保存上次移动时间用于防抖检测
	lastMoveTime := windowInfo.MoveTime
	windowInfo.MoveTime = time.Now()

	if windowInfo.IsSnapped {
		// 已吸附窗口：检查是否被用户拖拽解除吸附
		wss.handleSnappedWindow(window, windowInfo, currentPos)
		// 对于已吸附窗口，总是更新为当前位置
		windowInfo.LastPos = currentPos
	} else {
		// 未吸附窗口：检查是否应该吸附
		isSnapped := wss.handleUnsnappedWindow(window, windowInfo, currentPos, lastMoveTime)
		if !isSnapped {
			// 如果没有吸附，更新为当前位置
			windowInfo.LastPos = currentPos
		}
		// 如果成功吸附，位置已在handleUnsnappedWindow中更新
	}
}

// updateSnappedWindowPosition 更新已吸附窗口的位置
// 该函数根据主窗口的新位置和窗口的偏移量，计算并设置吸附窗口的目标位置
// 参数:
//
//	windowInfo - 包含窗口信息的结构体指针，用于获取窗口的偏移量和更新最后位置记录
func (wss *WindowSnapService) updateSnappedWindowPosition(windowInfo *models.WindowInfo) {
	// 计算新的目标位置（基于主窗口新位置）
	expectedX := wss.lastMainWindowPos.X + windowInfo.SnapOffset.X
	expectedY := wss.lastMainWindowPos.Y + windowInfo.SnapOffset.Y

	// 查找对应的window对象
	window, exists := wss.windowRefs[windowInfo.DocumentID]
	if !exists {
		return
	}

	// 设置更新标志，防止事件循环
	wss.isUpdatingPosition[windowInfo.DocumentID] = true

	wss.mu.Unlock()
	window.SetPosition(expectedX, expectedY)
	wss.mu.Lock()

	// 清除更新标志
	wss.isUpdatingPosition[windowInfo.DocumentID] = false

	windowInfo.LastPos = models.WindowPosition{X: expectedX, Y: expectedY}
}

// handleSnappedWindow 处理已吸附窗口的移动
// handleSnappedWindow 处理已吸附窗口的位置变化
// 当窗口被用户拖拽时，根据位置偏移和时间间隔判断是否为主动拖拽，如果是则解除窗口的吸附状态
//
// 参数:
//
//	window: Webview窗口对象指针
//	windowInfo: 窗口信息结构体指针，包含窗口的吸附状态、偏移量等信息
//	currentPos: 窗口当前位置坐标
func (wss *WindowSnapService) handleSnappedWindow(window *application.WebviewWindow, windowInfo *models.WindowInfo, currentPos models.WindowPosition) {
	// 计算预期位置
	expectedX := wss.lastMainWindowPos.X + windowInfo.SnapOffset.X
	expectedY := wss.lastMainWindowPos.Y + windowInfo.SnapOffset.Y

	// 计算实际位置与预期位置的距离
	distanceX := math.Abs(float64(currentPos.X - expectedX))
	distanceY := math.Abs(float64(currentPos.Y - expectedY))
	maxDistance := math.Max(distanceX, distanceY)

	// 用户拖拽检测：距离超过阈值且移动很快（使用统一的拖拽检测阈值）
	userDragThreshold := float64(wss.calculateAdaptiveThreshold())
	isUserDrag := maxDistance > userDragThreshold && time.Since(windowInfo.MoveTime) < dragDetectionThreshold

	if isUserDrag {
		// 用户主动拖拽，解除吸附
		windowInfo.IsSnapped = false
		windowInfo.SnapEdge = models.SnapEdgeNone
	}
}

// handleUnsnappedWindow 处理未吸附窗口的移动，返回是否成功吸附
// handleUnsnappedWindow 处理未吸附窗口的移动逻辑，检查是否应该将窗口吸附到主窗口边缘
// 参数:
//
//	window: Webview窗口对象，表示需要处理的窗口
//	windowInfo: 窗口信息结构体，包含窗口的状态和位置信息
//	currentPos: 当前窗口位置信息
//	lastMoveTime: 上次移动的时间戳
//
// 返回值:
//
//	bool: 如果窗口被成功吸附则返回true，否则返回false
func (wss *WindowSnapService) handleUnsnappedWindow(window *application.WebviewWindow, windowInfo *models.WindowInfo, currentPos models.WindowPosition, lastMoveTime time.Time) bool {
	// 检查是否应该吸附
	should, snapEdge := wss.shouldSnapToMainWindow(window, windowInfo, currentPos, lastMoveTime)
	if should {
		// 设置吸附状态
		windowInfo.IsSnapped = true
		windowInfo.SnapEdge = snapEdge

		// 执行吸附移动
		targetPos := wss.calculateSnapPosition(snapEdge, currentPos, windowInfo.DocumentID, window)

		// 设置更新标志，防止事件循环
		wss.isUpdatingPosition[windowInfo.DocumentID] = true

		wss.mu.Unlock()
		window.SetPosition(targetPos.X, targetPos.Y)
		wss.mu.Lock()

		// 清除更新标志
		wss.isUpdatingPosition[windowInfo.DocumentID] = false

		// 计算并保存偏移量
		windowInfo.SnapOffset.X = targetPos.X - wss.lastMainWindowPos.X
		windowInfo.SnapOffset.Y = targetPos.Y - wss.lastMainWindowPos.Y

		// 更新位置为吸附后的位置
		windowInfo.LastPos = targetPos

		return true
	}

	return false
}

// shouldSnapToMainWindow 吸附检测
// shouldSnapToMainWindow 判断给定的子窗口是否应该吸附到主窗口的某个边缘或角落。
// 参数：
//   - window: 当前正在移动的 WebviewWindow 实例
//   - windowInfo: 子窗口的信息模型对象
//   - currentPos: 子窗口当前的位置信息
//   - lastMoveTime: 上次移动事件的时间戳，用于防抖处理
//
// 返回值：
//   - bool: 是否需要进行吸附操作
//   - models.SnapEdge: 吸附的目标边缘类型（如顶部、底部、左侧等），如果不吸附则为 SnapEdgeNone
func (wss *WindowSnapService) shouldSnapToMainWindow(window *application.WebviewWindow, windowInfo *models.WindowInfo, currentPos models.WindowPosition, lastMoveTime time.Time) (bool, models.SnapEdge) {
	// 防抖：如果距离上次移动时间过短，则跳过检测以避免频繁触发
	timeSinceLastMove := time.Since(lastMoveTime)
	if timeSinceLastMove < debounceThreshold {
		return false, models.SnapEdgeNone
	}

	// 使用缓存的主窗口位置和尺寸数据。若尚未初始化，则立即更新缓存
	if wss.lastMainWindowSize[0] == 0 || wss.lastMainWindowSize[1] == 0 {
		wss.updateMainWindowCacheLocked()
	}

	mainPos := wss.lastMainWindowPos
	mainWidth := wss.lastMainWindowSize[0]
	mainHeight := wss.lastMainWindowSize[1]

	// 获取并使用缓存中的子窗口尺寸，减少系统调用开销
	windowWidth, windowHeight := wss.getWindowSizeCached(windowInfo.DocumentID, window)

	// 根据自适应逻辑计算吸附阈值，提高不同分辨率下的兼容性
	threshold := float64(wss.calculateAdaptiveThreshold())
	cornerThreshold := threshold * 1.5

	// 计算主窗口与子窗口各自的左右上下边界坐标
	mainLeft, mainTop := mainPos.X, mainPos.Y
	mainRight, mainBottom := mainPos.X+mainWidth, mainPos.Y+mainHeight

	windowLeft, windowTop := currentPos.X, currentPos.Y
	windowRight, windowBottom := currentPos.X+windowWidth, currentPos.Y+windowHeight

	// 定义一个结构体用于记录可能发生的吸附情况及其优先级
	type snapCheck struct {
		edge     models.SnapEdge
		distance float64
		priority int // 优先级：1 表示角落吸附，2 表示边缘吸附
	}

	var bestSnap *snapCheck

	// 先检查四个角落方向的吸附可能性（高优先级）
	cornerChecks := []struct {
		edge models.SnapEdge
		dx   int
		dy   int
	}{
		{models.SnapEdgeTopRight, mainRight - windowLeft, mainTop - windowBottom},
		{models.SnapEdgeBottomRight, mainRight - windowLeft, mainBottom - windowTop},
		{models.SnapEdgeBottomLeft, mainLeft - windowRight, mainBottom - windowTop},
		{models.SnapEdgeTopLeft, mainLeft - windowRight, mainTop - windowBottom},
	}

	for _, check := range cornerChecks {
		dist := math.Sqrt(float64(check.dx*check.dx + check.dy*check.dy))
		if dist <= cornerThreshold {
			if bestSnap == nil || dist < bestSnap.distance {
				bestSnap = &snapCheck{check.edge, dist, 1}
			}
		}
	}

	// 若无合适的角落吸附点，则继续检查四条边的边缘吸附（低优先级）
	if bestSnap == nil {
		edgeChecks := []struct {
			edge     models.SnapEdge
			distance float64
		}{
			{models.SnapEdgeRight, math.Abs(float64(mainRight - windowLeft))},
			{models.SnapEdgeLeft, math.Abs(float64(mainLeft - windowRight))},
			{models.SnapEdgeBottom, math.Abs(float64(mainBottom - windowTop))},
			{models.SnapEdgeTop, math.Abs(float64(mainTop - windowBottom))},
		}

		for _, check := range edgeChecks {
			if check.distance <= threshold {
				if bestSnap == nil || check.distance < bestSnap.distance {
					bestSnap = &snapCheck{check.edge, check.distance, 2}
				}
			}
		}
	}

	// 如果没有任何满足条件的吸附目标，则不执行吸附
	if bestSnap == nil {
		return false, models.SnapEdgeNone
	}

	// 返回应启用吸附，并告知具体吸附的方向
	return true, bestSnap.edge
}

// calculateSnapPosition 计算吸附目标位置
// calculateSnapPosition 根据指定的吸附边缘计算窗口的新位置。
// 参数:
//   - snapEdge: 指定窗口要吸附到主窗口的哪个边缘或角落。
//   - currentPos: 当前窗口的位置信息。
//   - documentID: 窗口对应的文档 ID，用于缓存尺寸查询。
//   - window: Webview 窗口对象，用于获取窗口尺寸等信息。
//
// 返回值:
//   - models.WindowPosition: 计算后的新窗口位置。
func (wss *WindowSnapService) calculateSnapPosition(snapEdge models.SnapEdge, currentPos models.WindowPosition, documentID int64, window *application.WebviewWindow) models.WindowPosition {
	// 使用缓存的主窗口信息
	mainPos := wss.lastMainWindowPos
	mainWidth := wss.lastMainWindowSize[0]
	mainHeight := wss.lastMainWindowSize[1]

	// 使用缓存的子窗口尺寸，减少系统调用
	windowWidth, windowHeight := wss.getWindowSizeCached(documentID, window)

	switch snapEdge {
	case models.SnapEdgeRight:
		return models.WindowPosition{
			X: mainPos.X + mainWidth,
			Y: currentPos.Y, // 保持当前Y位置
		}
	case models.SnapEdgeLeft:
		return models.WindowPosition{
			X: mainPos.X - windowWidth,
			Y: currentPos.Y,
		}
	case models.SnapEdgeBottom:
		return models.WindowPosition{
			X: currentPos.X,
			Y: mainPos.Y + mainHeight,
		}
	case models.SnapEdgeTop:
		return models.WindowPosition{
			X: currentPos.X,
			Y: mainPos.Y - windowHeight,
		}
	case models.SnapEdgeTopRight:
		return models.WindowPosition{
			X: mainPos.X + mainWidth,
			Y: mainPos.Y - windowHeight,
		}
	case models.SnapEdgeBottomRight:
		return models.WindowPosition{
			X: mainPos.X + mainWidth,
			Y: mainPos.Y + mainHeight,
		}
	case models.SnapEdgeBottomLeft:
		return models.WindowPosition{
			X: mainPos.X - windowWidth,
			Y: mainPos.Y + mainHeight,
		}
	case models.SnapEdgeTopLeft:
		return models.WindowPosition{
			X: mainPos.X - windowWidth,
			Y: mainPos.Y - windowHeight,
		}
	}

	return currentPos
}

// Cleanup 清理窗口快照服务的所有资源
// 该函数会清理主窗口和所有子窗口的事件监听器，并清空所有管理的窗口数据
// 此方法是线程安全的，会在执行过程中获取并释放互斥锁
func (wss *WindowSnapService) Cleanup() {
	wss.mu.Lock()
	defer wss.mu.Unlock()

	// 清理主窗口事件监听
	wss.cleanupMainWindowEvents()

	// 清理所有子窗口事件监听
	for documentID, unhook := range wss.windowMoveUnhooks {
		if unhook != nil {
			unhook()
		}
		delete(wss.windowMoveUnhooks, documentID)
	}

	// 清空管理的窗口
	wss.managedWindows = make(map[int64]*models.WindowInfo)
	wss.windowRefs = make(map[int64]*application.WebviewWindow)
	wss.windowSizeCache = make(map[int64][2]int)
	wss.isUpdatingPosition = make(map[int64]bool)
	wss.windowMoveUnhooks = make(map[int64]func())
}

// ServiceShutdown 实现服务关闭接口
// ServiceShutdown 关闭窗口快照服务
// 该函数负责清理服务资源，包括取消配置观察者和执行清理操作
//
// 返回值:
//
//	error - 返回nil，表示关闭操作成功
func (wss *WindowSnapService) ServiceShutdown() error {
	// 取消配置观察者
	if wss.cancelObserver != nil {
		wss.cancelObserver()
	}
	wss.Cleanup()
	return nil
}
