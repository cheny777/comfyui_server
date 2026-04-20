package comfyui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"comfyui_middleground/logger"

	"github.com/gorilla/websocket"
)

type ComfyUIClient struct {
	conn               *websocket.Conn
	httpClient         *http.Client
	host               string
	clientID           string                       // 存储client_id，确保WebSocket和HTTP请求使用同一个
	taskCallbacks      map[string]func(*TaskStatus) // 存储每个任务的回调函数
	taskContexts       map[string]*taskListener     // 存储每个任务的上下文
	mu                 sync.RWMutex
	messageLoopStarted bool // 标记消息循环是否已启动
	messageLoopMu      sync.Mutex
}

type TaskStatus struct {
	PromptID string   `json:"prompt_id"`
	Status   string   `json:"status"`
	Progress int      `json:"progress"`
	Images   []string `json:"images"`
	Error    string   `json:"error,omitempty"`
}

type ComfyUIMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type Node struct {
	ClassType string                 `json:"class_type"`
	Inputs    map[string]interface{} `json:"inputs"`
}

type Workflow map[string]Node

// 任务监听上下文
type taskListener struct {
	promptID        string
	callback        func(*TaskStatus)
	lastProgress    int
	collectedImages map[string]bool
	mu              sync.RWMutex
}

// 全局单例客户端
var (
	globalClient   *ComfyUIClient
	globalClientMu sync.RWMutex
)

// GetGlobalClient 获取全局ComfyUI客户端单例
func GetGlobalClient() *ComfyUIClient {
	globalClientMu.RLock()
	defer globalClientMu.RUnlock()
	return globalClient
}

// InitGlobalClient 初始化全局ComfyUI客户端（只初始化一次）
func InitGlobalClient(host string, timeout time.Duration) (*ComfyUIClient, error) {
	globalClientMu.Lock()
	defer globalClientMu.Unlock()

	// 如果已经初始化，检查连接状态
	if globalClient != nil {
		globalClient.mu.RLock()
		conn := globalClient.conn
		messageLoopRunning := globalClient.messageLoopStarted
		globalClient.mu.RUnlock()

		// 如果连接存在，直接返回（消息循环会在需要时自动重启）
		if conn != nil {
			logger.Debugf("[ComfyUI] 使用已存在的全局客户端 | 连接状态=已连接 | 消息循环=%v", messageLoopRunning)
			return globalClient, nil
		}

		// 如果连接断开，尝试重连
		logger.Warnf("[ComfyUI] ⚠️ 检测到连接断开，尝试重新连接")
		if err := globalClient.Connect(); err != nil {
			logger.Errorf("[ComfyUI] ❌ 重连失败: %v，将创建新客户端", err)
			// 重连失败，创建新客户端
			globalClient = nil
		} else {
			logger.Infof("[ComfyUI] ✅ 重连成功")
			return globalClient, nil
		}
	}

	// 创建新客户端
	globalClient = &ComfyUIClient{
		httpClient:    &http.Client{Timeout: timeout},
		host:          host,
		taskCallbacks: make(map[string]func(*TaskStatus)),
		taskContexts:  make(map[string]*taskListener),
	}

	// 连接WebSocket
	if err := globalClient.Connect(); err != nil {
		globalClient = nil
		return nil, fmt.Errorf("连接ComfyUI失败: %w", err)
	}

	logger.Infof("[ComfyUI] 全局客户端初始化成功")
	return globalClient, nil
}

// NewComfyUIClient 创建新的ComfyUI客户端（不推荐使用，建议使用InitGlobalClient）
func NewComfyUIClient(host string, timeout time.Duration) *ComfyUIClient {
	logger.Warnf("[ComfyUI] 警告: 使用NewComfyUIClient创建新实例，建议使用InitGlobalClient获取全局单例")
	return &ComfyUIClient{
		httpClient:    &http.Client{Timeout: timeout},
		host:          host,
		taskCallbacks: make(map[string]func(*TaskStatus)),
		taskContexts:  make(map[string]*taskListener),
	}
}

func (c *ComfyUIClient) Connect() error {
	// 如果已经连接，直接返回
	if c.conn != nil {
		logger.Debugf("[ComfyUI] WebSocket连接已存在，跳过重复连接")
		return nil
	}

	// 关键修复：生成并保存client_id，确保WebSocket和HTTP请求使用同一个
	c.clientID = fmt.Sprintf("go-client-%d", time.Now().Unix())
	url := fmt.Sprintf("ws://%s/ws?clientId=%s", c.host, c.clientID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	c.conn = conn
	logger.Infof("[ComfyUI] 已连接到 ComfyUI: %s", url)

	// 关键修复：不在这里启动消息循环，而是在第一个任务注册时启动
	// 这样可以避免在任务注册之前就收到消息（参考demo：在ListenForImages中才开始读取）
	logger.Debugf("[ComfyUI] WebSocket连接已建立，消息循环将在第一个任务注册时启动")

	return nil
}

func (c *ComfyUIClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *ComfyUIClient) QueuePrompt(workflow Workflow) (string, error) {
	// 关键修复：使用与WebSocket连接相同的client_id
	if c.clientID == "" {
		c.clientID = fmt.Sprintf("go-client-%d", time.Now().Unix())
		logger.Warnf("[ComfyUI] client_id未设置，使用新生成的: %s", c.clientID)
	}

	promptData := map[string]interface{}{
		"prompt":    workflow,
		"client_id": c.clientID, // 使用与WebSocket连接相同的client_id
	}

	logger.Debugf("[ComfyUI] 提交任务，使用client_id: %s", c.clientID)

	url := fmt.Sprintf("http://%s/prompt", c.host)
	jsonData, err := json.Marshal(promptData)
	if err != nil {
		return "", fmt.Errorf("序列化失败: %w", err)
	}

	resp, err := c.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	promptID, ok := result["prompt_id"].(string)
	if !ok {
		return "", fmt.Errorf("未找到 prompt_id")
	}

	logger.Infof("📤 任务已提交，Prompt ID: %s", promptID)
	return promptID, nil
}

func (c *ComfyUIClient) ListenForTask(promptID string, callback func(*TaskStatus)) error {
	logger.Infof("[ComfyUI] 注册任务监听: PromptID=%s", promptID)

	// 检查连接状态，如果连接断开则尝试重连
	c.mu.RLock()
	conn := c.conn
	messageLoopRunning := c.messageLoopStarted
	c.mu.RUnlock()

	if conn == nil {
		logger.Warnf("[ComfyUI] ⚠️ WebSocket连接已断开，尝试重新连接")
		if err := c.Connect(); err != nil {
			return fmt.Errorf("WebSocket连接断开且重连失败: %w", err)
		}
		// 重新获取连接状态
		c.mu.RLock()
		conn = c.conn
		c.mu.RUnlock()
		if conn == nil {
			return fmt.Errorf("重连后连接仍为空")
		}
		logger.Infof("[ComfyUI] ✅ WebSocket重连成功")
	}

	// 如果消息循环已退出，需要重新启动
	if !messageLoopRunning {
		logger.Warnf("[ComfyUI] ⚠️ 消息循环已停止，将重新启动")
		// 重置标志，让下面的逻辑重新启动消息循环
		c.messageLoopMu.Lock()
		c.messageLoopStarted = false
		c.messageLoopMu.Unlock()
	}

	// 注册任务监听器和上下文（参考demo实现：所有任务共享同一个WebSocket连接）
	c.mu.Lock()
	c.taskCallbacks[promptID] = callback
	c.taskContexts[promptID] = &taskListener{
		promptID:        promptID,
		callback:        callback,
		lastProgress:    -1,
		collectedImages: make(map[string]bool),
	}
	activeCount := len(c.taskContexts)
	c.mu.Unlock()

	// 关键修复：确保消息循环已启动（延迟启动，参考demo：在需要时才启动）
	c.messageLoopMu.Lock()
	if !c.messageLoopStarted {
		// 再次检查连接（可能在重连后）
		c.mu.RLock()
		conn = c.conn
		c.mu.RUnlock()

		if conn == nil {
			c.messageLoopMu.Unlock()
			logger.Errorf("[ComfyUI] ❌ WebSocket连接未建立，尝试重新连接")
			// 尝试重新连接
			if err := c.Connect(); err != nil {
				return fmt.Errorf("WebSocket连接未建立且重连失败: %w", err)
			}
			// 重新获取锁并检查
			c.messageLoopMu.Lock()
			c.mu.RLock()
			conn = c.conn
			c.mu.RUnlock()
		}

		// 再次检查连接是否有效（防止在检查后连接断开）
		if conn == nil {
			c.messageLoopMu.Unlock()
			return fmt.Errorf("WebSocket连接未建立，无法启动消息循环")
		}

		// 验证连接是否真正可用（通过设置一个很短的读取超时来测试）
		testDeadline := time.Now().Add(100 * time.Millisecond)
		if err := conn.SetReadDeadline(testDeadline); err != nil {
			c.messageLoopMu.Unlock()
			logger.Errorf("[ComfyUI] ❌ WebSocket连接无效，无法设置读取超时: %v", err)
			// 尝试重新连接
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			if err := c.Connect(); err != nil {
				return fmt.Errorf("WebSocket连接无效且重连失败: %w", err)
			}
			c.messageLoopMu.Lock()
			c.mu.RLock()
			conn = c.conn
			c.mu.RUnlock()
		}
		// 重置超时
		if conn != nil {
			conn.SetReadDeadline(time.Time{})
		}

		c.messageLoopStarted = true
		logger.Infof("[ComfyUI] 🔄 启动全局消息循环（第一个任务注册时）")
		go c.messageLoop()
		// 给消息循环一点时间启动
		time.Sleep(50 * time.Millisecond)
		logger.Debugf("[ComfyUI] ✅ 消息循环已启动")
	}
	c.messageLoopMu.Unlock()

	logger.Debugf("[ComfyUI] ✅ 任务监听已注册: PromptID=%s, 当前活跃任务数=%d", promptID, activeCount)
	return nil
}

// UnregisterTask 取消注册任务监听器
func (c *ComfyUIClient) UnregisterTask(promptID string) {
	c.mu.Lock()
	delete(c.taskCallbacks, promptID)
	delete(c.taskContexts, promptID)
	c.mu.Unlock()
	logger.Debugf("[ComfyUI] 任务监听已取消注册: PromptID=%s", promptID)
}

// MigrateTaskListener 将任务监听器从一个promptID迁移到另一个
func (c *ComfyUIClient) MigrateTaskListener(oldPromptID, newPromptID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx, exists := c.taskContexts[oldPromptID]
	if !exists {
		return fmt.Errorf("源任务监听器不存在: %s", oldPromptID)
	}

	callback, exists := c.taskCallbacks[oldPromptID]
	if !exists {
		return fmt.Errorf("源回调函数不存在: %s", oldPromptID)
	}

	// 更新promptID
	ctx.promptID = newPromptID

	// 迁移到新的promptID
	c.taskContexts[newPromptID] = ctx
	c.taskCallbacks[newPromptID] = callback

	// 删除旧的
	delete(c.taskContexts, oldPromptID)
	delete(c.taskCallbacks, oldPromptID)

	logger.Debugf("[ComfyUI] ✅ 任务监听器已迁移: %s -> %s", oldPromptID, newPromptID)
	return nil
}

// 全局消息循环（参考demo实现：只有一个循环读取所有消息）
func (c *ComfyUIClient) messageLoop() {
	logger.Infof("[ComfyUI] ════════════════════════════════════════════════════")
	logger.Infof("[ComfyUI] 🔄 全局消息循环启动")
	logger.Infof("[ComfyUI] ════════════════════════════════════════════════════")

	defer func() {
		// 捕获 panic，防止程序崩溃
		if r := recover(); r != nil {
			logger.Errorf("[ComfyUI] WebSocket消息循环发生panic: %v", r)
		}

		// 标记消息循环已停止
		c.mu.Lock()
		c.messageLoopStarted = false
		c.mu.Unlock()

		logger.Infof("[ComfyUI] 全局消息循环退出")
	}()

	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			logger.Errorf("[ComfyUI] WebSocket连接为空，退出消息循环")
			return
		}

		// 设置读取超时（60秒，用于检测连接是否存活）
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[ComfyUI] 设置读取超时失败: %v", err)
			// 连接可能已失效，标记连接为nil并退出
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()
			return
		}

		// 使用recover保护ReadMessage调用，防止panic
		var messageType int
		var message []byte
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[ComfyUI] ReadMessage发生panic: %v", r)
					err = fmt.Errorf("read panic: %v", r)
					// panic后标记连接为nil
					c.mu.Lock()
					c.conn = nil
					c.mu.Unlock()
				}
			}()
			messageType, message, err = conn.ReadMessage()
		}()

		if err != nil {
			// 检查是否是超时错误（这是唯一可以继续的错误）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				// 超时是正常的，继续循环等待
				logger.Debugf("[ComfyUI] 读取超时，继续等待消息")
				continue
			}

			// 检查是否是正常的关闭错误
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 这是意外的关闭错误
				logger.Warnf("[ComfyUI] WebSocket连接意外关闭: %v", err)
			} else {
				// 其他所有错误（包括正常关闭、EOF、连接重置等）都应该退出
				logger.Warnf("[ComfyUI] 读取消息失败，连接已断开: %v", err)
			}

			// 标记连接为nil，避免后续继续使用
			c.mu.Lock()
			c.conn = nil
			c.mu.Unlock()

			// 无论什么错误，都退出循环，避免重复读取失败的连接
			// 注意：如果有活跃任务，它们会在下次提交任务时触发重连
			return
		}

		// 收到消息后重置超时
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[ComfyUI] 重置读取超时失败: %v", err)
			return
		}

		// 调试：打印所有收到的原始消息（DEBUG级别）
		logger.Debugf("[ComfyUI] 🔍 收到原始消息: Type=%d, Length=%d", messageType, len(message))

		if messageType != websocket.TextMessage {
			continue
		}

		messageStr := strings.TrimSpace(string(message))
		if len(messageStr) == 0 || strings.Contains(messageStr, "\x00") {
			continue
		}

		var msg ComfyUIMessage
		if err := json.Unmarshal([]byte(messageStr), &msg); err != nil {
			logger.Warnf("[ComfyUI] 解析消息失败: Error=%v, 原始消息长度=%d", err, len(messageStr))
			// 打印原始消息的前100个字符用于调试
			if len(messageStr) > 100 {
				logger.Warnf("[ComfyUI] 原始消息前100字符: %s", messageStr[:100])
			} else {
				logger.Warnf("[ComfyUI] 原始消息: %s", messageStr)
			}
			continue
		}

		// 根据消息类型决定日志级别
		// status消息太多，只在DEBUG级别打印
		if msg.Type == "status" {
			logger.Debugf("[ComfyUI] 📨 收到消息: Type=%s", msg.Type)
		} else {
			// 关键消息在INFO级别打印
			logger.Infof("[ComfyUI] 📨 收到消息: Type=%s", msg.Type)
		}

		// 关键消息的详细内容在DEBUG级别打印（减少日志噪音）
		if logger.GetLevel() == logger.DEBUG {
			if dataJSON, err := json.Marshal(msg.Data); err == nil {
				logger.Debugf("[ComfyUI] 📨 消息内容: Type=%s, Data=%s", msg.Type, string(dataJSON))
			}
		}

		// 获取所有活跃的任务（参考demo：所有任务都接收所有消息）
		c.mu.RLock()
		activeTasks := make(map[string]*taskListener)
		for pid, ctx := range c.taskContexts {
			activeTasks[pid] = ctx
		}
		activeTaskCount := len(activeTasks)
		c.mu.RUnlock()

		// 活跃任务信息只在DEBUG级别打印（减少日志噪音）
		if activeTaskCount > 0 && logger.GetLevel() == logger.DEBUG {
			taskIDs := make([]string, 0, activeTaskCount)
			for pid := range activeTasks {
				taskIDs = append(taskIDs, pid)
			}
			logger.Debugf("[ComfyUI] 📋 当前活跃任务数=%d, 任务列表=%v", activeTaskCount, taskIDs)
		}

		if len(activeTasks) == 0 {
			// 对于关键消息类型，即使没有活跃任务也记录日志（用于调试）
			if msg.Type == "execution_start" || msg.Type == "progress" || msg.Type == "executed" || msg.Type == "execution_success" {
				logger.Warnf("[ComfyUI] ⚠️  收到关键消息但没有活跃任务: Type=%s", msg.Type)
				if dataJSON, err := json.Marshal(msg.Data); err == nil {
					logger.Warnf("[ComfyUI] 消息内容: %s", string(dataJSON))
				}
				logger.Warnf("[ComfyUI] ⚠️  这通常意味着任务监听器注册太晚，消息已错过")
			} else {
				logger.Debugf("[ComfyUI] ⚠️  没有活跃的任务监听器，跳过消息: Type=%s", msg.Type)
			}
			continue
		}

		// 处理消息并分发给所有活跃的任务（参考demo实现）
		switch msg.Type {
		case "execution_cached":
			// execution_cached消息（参考demo实现）
			logger.Debugf("[ComfyUI] ⚡ 执行已缓存（使用缓存结果）")
			// 缓存执行也需要通知任务，但通常很快完成
			for promptID := range activeTasks {
				logger.Debugf("[ComfyUI]   缓存执行: PromptID=%s", promptID)
				// 不更新状态，等待后续消息
			}

		case "status":
			// status消息是全局的，不需要分发给特定任务
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if execStatus, ok := data["status"].(map[string]interface{}); ok {
					if execInfo, ok := execStatus["exec_info"].(map[string]interface{}); ok {
						if queueRemaining, ok := execInfo["queue_remaining"].(float64); ok {
							logger.Debugf("[ComfyUI] 队列状态: 队列剩余=%d", int(queueRemaining))
						}
					}
				}
			}

		case "execution_start":
			// execution_start消息分发给所有活跃任务（参考demo实现）
			for promptID, ctx := range activeTasks {
				ctx.mu.Lock()
				ctx.lastProgress = 0
				ctx.mu.Unlock()

				logger.Infof("[ComfyUI] 🚀 执行开始 | PromptID=%s | 进度: %s | 0%%", promptID, getProgressBar(0))

				status := &TaskStatus{
					PromptID: promptID,
					Status:   "running",
					Progress: 0,
				}
				ctx.callback(status)
			}

		case "progress":
			// progress消息分发给所有活跃任务（参考demo实现）
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if value, ok := data["value"].(float64); ok {
					if max, ok := data["max"].(float64); ok {
						progress := int((value / max) * 100)

						for promptID, ctx := range activeTasks {
							ctx.mu.Lock()
							lastProgress := ctx.lastProgress
							if progress != lastProgress {
								ctx.lastProgress = progress
								ctx.mu.Unlock()

								// 打印进度条（每5%打印一次，减少日志噪音）
								if progress%5 == 0 || progress == 100 {
									progressBar := getProgressBar(progress)
									logger.Infof("[ComfyUI进度] PromptID=%s | %s | %d%%", promptID, progressBar, progress)
								} else {
									logger.Debugf("[ComfyUI进度] PromptID=%s | %d%%", promptID, progress)
								}

								status := &TaskStatus{
									PromptID: promptID,
									Status:   "running",
									Progress: progress,
								}
								ctx.callback(status)
							} else {
								ctx.mu.Unlock()
							}
						}
					}
				}
			}

		case "executed":
			// executed消息分发给所有活跃任务（参考demo实现）
			if data, ok := msg.Data.(map[string]interface{}); ok {
				nodeIDFloat, ok := data["node"].(float64)
				if !ok {
					logger.Warnf("[ComfyUI] executed消息中没有node字段: Data=%v", data)
					continue
				}
				nodeID := int(nodeIDFloat)

				for promptID, ctx := range activeTasks {
					logger.Debugf("[ComfyUI] ✓ 节点执行完成: PromptID=%s, NodeID=%d", promptID, nodeID)

					// 检查是否有图像输出（参考demo实现）
					if outputs, ok := data["output"].(map[string]interface{}); ok {
						if images, ok := outputs["images"].([]interface{}); ok && len(images) > 0 {
							logger.Debugf("[ComfyUI] 检测到图像: PromptID=%s, NodeID=%d, 图像数量=%d", promptID, nodeID, len(images))

							ctx.mu.Lock()
							var imageURLs []string
							for i, img := range images {
								if imgMap, ok := img.(map[string]interface{}); ok {
									filename, _ := imgMap["filename"].(string)
									if filename != "" && !ctx.collectedImages[filename] {
										imageURLs = append(imageURLs, filename)
										ctx.collectedImages[filename] = true
										logger.Debugf("[ComfyUI] 收集图像[%d/%d]: PromptID=%s, Filename=%s",
											i+1, len(images), promptID, filename)
									}
								}
							}
							lastProgress := ctx.lastProgress
							ctx.mu.Unlock()

							if len(imageURLs) > 0 {
								logger.Debugf("[ComfyUI] 发送图像状态: PromptID=%s, Images=%v", promptID, imageURLs)
								status := &TaskStatus{
									PromptID: promptID,
									Status:   "running",
									Images:   imageURLs,
									Progress: lastProgress,
								}
								ctx.callback(status)
							}
						}
					}
				}
			}

		case "execution_success":
			// execution_success消息分发给所有活跃任务（参考demo实现）
			logger.Infof("[ComfyUI] ✅ 执行成功")

			for promptID, ctx := range activeTasks {
				ctx.mu.Lock()
				lastProgress := ctx.lastProgress
				if lastProgress < 0 {
					lastProgress = 100
					logger.Warnf("[ComfyUI] ⚠️  未收到progress消息，使用默认进度100%%: PromptID=%s", promptID)
				}
				allImages := make([]string, 0, len(ctx.collectedImages))
				for img := range ctx.collectedImages {
					allImages = append(allImages, img)
				}
				ctx.mu.Unlock()

				logger.Infof("[ComfyUI] ✅ 执行成功 | PromptID=%s | 进度: %s | %d%% | 图像数=%d",
					promptID, getProgressBar(lastProgress), lastProgress, len(allImages))
				if len(allImages) > 0 && logger.GetLevel() == logger.DEBUG {
					logger.Debugf("[ComfyUI]   图像列表: %v", allImages)
				}

				// 等待一段时间确保所有消息都处理完（参考demo实现）
				time.Sleep(2 * time.Second)

				// 如果没有收集到图像，尝试查询历史记录（参考demo实现）
				if len(allImages) == 0 {
					logger.Debugf("[ComfyUI] 未收集到图像，尝试查询历史记录: PromptID=%s", promptID)
					historyImages := c.queryHistoryForImages(promptID, ctx.collectedImages)
					if len(historyImages) > 0 {
						logger.Infof("[ComfyUI] 从历史记录找到图像: PromptID=%s, 图像数=%d", promptID, len(historyImages))
						allImages = historyImages
						// 发送图像状态
						status := &TaskStatus{
							PromptID: promptID,
							Status:   "running",
							Images:   allImages,
							Progress: lastProgress,
						}
						ctx.callback(status)
					}
				}

				status := &TaskStatus{
					PromptID: promptID,
					Status:   "completed",
					Progress: 100,
					Images:   allImages,
				}
				ctx.callback(status)

				// 移除已完成的任务
				c.mu.Lock()
				delete(c.taskCallbacks, promptID)
				delete(c.taskContexts, promptID)
				c.mu.Unlock()
				logger.Debugf("[ComfyUI] 任务监听已移除: PromptID=%s", promptID)
			}

		case "execution_error":
			// execution_error消息分发给所有活跃任务
			if data, ok := msg.Data.(map[string]interface{}); ok {
				errorMsg := fmt.Sprintf("%v", data)
				for promptID, ctx := range activeTasks {
					logger.Errorf("[ComfyUI] ❌ 执行错误 | PromptID=%s | Error=%s", promptID, errorMsg)
					status := &TaskStatus{
						PromptID: promptID,
						Status:   "failed",
						Error:    errorMsg,
					}
					ctx.callback(status)

					// 移除失败的任务
					c.mu.Lock()
					delete(c.taskCallbacks, promptID)
					delete(c.taskContexts, promptID)
					c.mu.Unlock()
				}
			}

		default:
			// 打印所有未处理的消息类型，用于调试
			logger.Warnf("[ComfyUI] ⚠️  未处理的消息类型: Type=%s", msg.Type)
			if dataJSON, err := json.MarshalIndent(msg.Data, "", "  "); err == nil {
				logger.Debugf("[ComfyUI] 消息数据:\n%s", string(dataJSON))
			}
			// 对于未知消息类型，也打印完整消息用于调试
			if fullJSON, err := json.MarshalIndent(msg, "", "  "); err == nil {
				logger.Debugf("[ComfyUI] 完整消息:\n%s", string(fullJSON))
			}
		}
	}
}

// 辅助函数：获取map的所有key
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// 获取进度条字符串
func getProgressBar(progress int) string {
	const barLength = 30
	filled := int(float64(barLength) * float64(progress) / 100)
	bar := ""
	for i := 0; i < barLength; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}

// 查询历史记录获取图像（参考demo实现）
func (c *ComfyUIClient) queryHistoryForImages(promptID string, collectedImages map[string]bool) []string {
	logger.Debugf("[ComfyUI] 查询历史记录: PromptID=%s", promptID)

	// 使用 /history 端点（参考demo实现）
	url := fmt.Sprintf("http://%s/history", c.host)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		logger.Debugf("[ComfyUI] 查询历史记录失败: PromptID=%s, Error=%v", promptID, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Debugf("[ComfyUI] 查询历史记录HTTP错误: PromptID=%s, StatusCode=%d", promptID, resp.StatusCode)
		return nil
	}

	var history map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		logger.Debugf("[ComfyUI] 解析历史记录失败: PromptID=%s, Error=%v", promptID, err)
		return nil
	}

	logger.Debugf("[ComfyUI] 历史记录查询成功: PromptID=%s", promptID)

	var images []string
	// 查找历史记录中对应 promptID 的输出（参考demo实现）
	if promptData, ok := history[promptID].(map[string]interface{}); ok {
		if outputs, ok := promptData["outputs"].(map[string]interface{}); ok {
			logger.Debugf("[ComfyUI] 从历史记录中查找图像: PromptID=%s", promptID)
			for nodeID, nodeOutput := range outputs {
				if outputMap, ok := nodeOutput.(map[string]interface{}); ok {
					if nodeImages, ok := outputMap["images"].([]interface{}); ok && len(nodeImages) > 0 {
						logger.Debugf("[ComfyUI] 在节点 %s 找到 %d 张图像: PromptID=%s", nodeID, len(nodeImages), promptID)
						for _, img := range nodeImages {
							if imgMap, ok := img.(map[string]interface{}); ok {
								filename, _ := imgMap["filename"].(string)
								if filename != "" && !collectedImages[filename] {
									images = append(images, filename)
									collectedImages[filename] = true
									logger.Debugf("[ComfyUI] 从历史记录找到图像: PromptID=%s, NodeID=%s, Filename=%s",
										promptID, nodeID, filename)
								}
							}
						}
					}
				}
			}
		} else {
			logger.Debugf("[ComfyUI] 历史记录中未找到 outputs 字段: PromptID=%s", promptID)
		}
	} else {
		logger.Debugf("[ComfyUI] 历史记录中未找到 prompt ID: PromptID=%s", promptID)
	}

	if len(images) == 0 {
		logger.Debugf("[ComfyUI] 历史记录中未找到图像: PromptID=%s", promptID)
	}

	return images
}

func (c *ComfyUIClient) DownloadImage(filename, subfolder, imgType, outputPath string) error {
	var url string
	if subfolder != "" {
		url = fmt.Sprintf("http://%s/view?filename=%s&subfolder=%s&type=%s",
			c.host, filename, subfolder, imgType)
	} else {
		url = fmt.Sprintf("http://%s/view?filename=%s&type=%s",
			c.host, filename, imgType)
	}

	logger.Debugf("[ComfyUI下载] 开始下载图像: Filename=%s, URL=%s, OutputPath=%s", filename, url, outputPath)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		logger.Debugf("[ComfyUI下载] HTTP请求失败: URL=%s, Error=%v", url, err)
		return fmt.Errorf("下载图像失败: %w", err)
	}
	defer resp.Body.Close()

	logger.Debugf("[ComfyUI下载] HTTP响应: URL=%s, StatusCode=%d, ContentLength=%d",
		url, resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		logger.Debugf("[ComfyUI下载] HTTP错误: URL=%s, StatusCode=%d", url, resp.StatusCode)
		return fmt.Errorf("下载图像失败: HTTP %d", resp.StatusCode)
	}

	// 创建输出目录
	dir := filepath.Dir(outputPath)
	logger.Debugf("[ComfyUI下载] 创建目录: %s", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Debugf("[ComfyUI下载] 创建目录失败: Path=%s, Error=%v", dir, err)
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 保存文件
	logger.Debugf("[ComfyUI下载] 创建文件: %s", outputPath)
	file, err := os.Create(outputPath)
	if err != nil {
		logger.Debugf("[ComfyUI下载] 创建文件失败: Path=%s, Error=%v", outputPath, err)
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, resp.Body)
	if err != nil {
		logger.Debugf("[ComfyUI下载] 写入文件失败: Path=%s, Error=%v", outputPath, err)
		return fmt.Errorf("写入文件失败: %w", err)
	}

	logger.Debugf("[ComfyUI下载] 图像下载成功: Filename=%s, Path=%s, Size=%d bytes", filename, outputPath, written)
	return nil
}

func ExtractPrompts(workflow Workflow) (promptText, negativePrompt string) {
	for _, node := range workflow {
		if node.ClassType == "CLIPTextEncode" {
			if text, ok := node.Inputs["text"].(string); ok {
				// 简单判断：通常第一个是正向，第二个是负向
				// 实际应该根据节点连接关系判断
				if promptText == "" {
					promptText = text
				} else if negativePrompt == "" {
					negativePrompt = text
				}
			}
		}
	}
	return promptText, negativePrompt
}
