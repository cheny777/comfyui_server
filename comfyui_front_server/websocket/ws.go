package websocket

import (
	"comfyui_front_server/config"
	"comfyui_front_server/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TaskUpdateCallback 任务更新回调函数类型
type TaskUpdateCallback func(taskID string, status string, progress int, promptID string, errorMsg string) error

// FileAddCallback 文件添加回调函数类型
type FileAddCallback func(taskID string, userID int64, fileInfo map[string]interface{}) error

var (
	taskUpdateCallback TaskUpdateCallback
	fileAddCallback    FileAddCallback
)

// SetTaskUpdateCallback 设置任务更新回调
func SetTaskUpdateCallback(callback TaskUpdateCallback) {
	taskUpdateCallback = callback
}

// SetFileAddCallback 设置文件添加回调
func SetFileAddCallback(callback FileAddCallback) {
	fileAddCallback = callback
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境应该限制
	},
}

// MiddleConnection 中台连接
type MiddleConnection struct {
	conn            *websocket.Conn
	serverID        string
	connected       bool
	mu              sync.RWMutex
	userConnections map[int64]*UserConnection // userID -> UserConnection
}

// UserConnection 用户连接（用于WebSocket推送）
type UserConnection struct {
	conn      *websocket.Conn
	userID    int64
	connected bool
	mu        sync.RWMutex
}

var (
	middleConnections = make(map[string]*MiddleConnection) // serverID -> MiddleConnection
	middleConnMu      sync.RWMutex
	userConnections   = make(map[int64]*UserConnection) // userID -> UserConnection
	userConnMu        sync.RWMutex
	// 响应通道映射：requestID -> response channel
	responseChannels = make(map[string]chan *WSMessage)
	responseChMu     sync.RWMutex
)

// HandleMiddleConnection 处理中台WebSocket连接
func HandleMiddleConnection(c *gin.Context) {
	logger.Infof("[WebSocket] 📥 收到中台WebSocket连接请求")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf("[WebSocket] ❌ WebSocket升级失败: %v", err)
		return
	}

	// 生成临时连接ID（如果没有serverID，使用连接地址+时间戳）
	connID := fmt.Sprintf("conn_%d", time.Now().UnixNano())

	middleConn := &MiddleConnection{
		conn:            conn,
		connected:       true,
		serverID:        connID, // 使用临时ID
		userConnections: make(map[int64]*UserConnection),
	}

	// 立即注册到连接池（不需要认证）
	middleConnMu.Lock()
	middleConnections[connID] = middleConn
	connCount := len(middleConnections)
	middleConnMu.Unlock()

	logger.Infof("[WebSocket] ✅ WebSocket连接已建立并注册: ConnID=%s | 当前连接数=%d", connID, connCount)

	// 设置读取超时（用于心跳检测，60秒超时）
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// 启动消息监听
	go middleConn.handleMessages()
}

func (mc *MiddleConnection) handleMessages() {
	defer func() {
		// 捕获 panic，防止程序崩溃
		if r := recover(); r != nil {
			logger.Errorf("[WebSocket] 中台WebSocket消息监听发生panic: %v", r)
		}

		mc.mu.Lock()
		mc.connected = false
		serverID := mc.serverID
		if mc.conn != nil {
			mc.conn.Close()
		}
		mc.mu.Unlock()

		// 从连接池中移除
		if serverID != "" {
			middleConnMu.Lock()
			delete(middleConnections, serverID)
			remainingCount := len(middleConnections)
			middleConnMu.Unlock()
			logger.Infof("[WebSocket] 中台连接已从连接池移除: ServerID=%s | 剩余连接数=%d", serverID, remainingCount)
		}

		logger.Info("中台WebSocket连接已关闭")
	}()

	for {
		// 设置读取超时（60秒，用于心跳检测）
		mc.mu.RLock()
		conn := mc.conn
		connected := mc.connected
		mc.mu.RUnlock()

		if conn == nil || !connected {
			return
		}

		// 每次读取前重置超时时间
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[WebSocket] 设置读取超时失败: %v", err)
			return
		}

		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			// 检查是否是超时错误（这是唯一可以继续的错误）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				// 超时是正常的，继续循环等待
				logger.Debugf("[WebSocket] 读取超时，继续等待消息")
				continue
			}

			// 检查是否是正常的关闭错误
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 这是意外的关闭错误
				logger.Warnf("[WebSocket] WebSocket连接意外关闭: %v", err)
			} else {
				// 其他所有错误（包括正常关闭、EOF、连接重置等）都应该退出
				logger.Warnf("[WebSocket] 读取消息失败，连接已断开: %v", err)
			}
			// 无论什么错误，都退出循环，避免重复读取失败的连接
			return
		}

		// 收到消息后重置超时
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[WebSocket] 重置读取超时失败: %v", err)
			return
		}

		mc.handleMessage(&msg)
	}
}

func (mc *MiddleConnection) handleMessage(msg *WSMessage) {
	logger.Debugf("[WebSocket] 处理中台消息: Type=%s | RequestID=%s | UserID=%d", msg.Type, msg.RequestID, msg.UserID)

	switch msg.Type {
	case "auth":
		mc.handleAuth(msg)
	case "heartbeat":
		mc.handleHeartbeat(msg)
	case "heartbeat_pong":
		// 心跳响应，记录日志即可
		logger.Debug("收到心跳响应")
	case "task_submitted":
		// task_submitted 是任务提交的响应，发送到响应通道
		logger.Debugf("[WebSocket] 收到任务提交响应: Type=%s | RequestID=%s", msg.Type, msg.RequestID)
		mc.handleQueryResponse(msg)
	case "task_progress", "task_complete", "task_failed", "image_ready":
		// 转发给用户
		logger.Debugf("[WebSocket] 转发消息给用户: Type=%s | RequestID=%s", msg.Type, msg.RequestID)
		mc.forwardToUser(msg)
	case "workflow_list_response", "task_status_response", "image_query_response", "image_file_response", "queue_query_response":
		// 查询响应消息，发送到响应通道
		logger.Debugf("[WebSocket] 收到查询响应: Type=%s | RequestID=%s", msg.Type, msg.RequestID)
		mc.handleQueryResponse(msg)
	case "error":
		// 错误消息也转发给用户或响应通道
		logger.Debugf("[WebSocket] 收到错误消息: RequestID=%s", msg.RequestID)
		if msg.RequestID != "" {
			mc.handleQueryResponse(msg)
		} else {
			mc.forwardToUser(msg)
		}
	default:
		logger.Warnf("[WebSocket] 未知消息类型: %s", msg.Type)
	}
}

func (mc *MiddleConnection) handleAuth(msg *WSMessage) {
	logger.Infof("[WebSocket] 📥 收到认证请求（可选）")

	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		logger.Debugf("[WebSocket] 认证消息格式错误，忽略: Data类型=%T", msg.Data)
		// 不返回错误，因为认证是可选的
		return
	}

	serverID, _ := data["server_id"].(string)
	secretKey, _ := data["secret_key"].(string)

	logger.Debugf("[WebSocket] 认证信息: ServerID=%s | SecretKey长度=%d", serverID, len(secretKey))

	// 如果提供了 serverID，更新连接ID
	if serverID != "" {
		oldServerID := mc.serverID

		mc.mu.Lock()
		mc.serverID = serverID
		mc.mu.Unlock()

		// 如果 serverID 改变，更新连接池中的 key
		if oldServerID != serverID {
			middleConnMu.Lock()
			// 删除旧的连接记录
			delete(middleConnections, oldServerID)
			// 添加新的连接记录
			middleConnections[serverID] = mc
			connCount := len(middleConnections)
			middleConnMu.Unlock()

			logger.Infof("[WebSocket] ✅ 更新连接ID: OldID=%s | NewID=%s | 当前连接数=%d",
				oldServerID, serverID, connCount)
		}

		// 如果提供了密钥，验证密钥（可选）
		if secretKey != "" {
			expectedSecret := config.GetMiddleSecretKey()
			if secretKey != expectedSecret {
				logger.Warnf("[WebSocket] ⚠️ 认证密钥不匹配，但连接仍然有效 | ServerID=%s", serverID)
			}
		}
	}

	// 发送认证成功响应
	if err := mc.sendMessage(&WSMessage{
		Type: "auth_response",
		Data: map[string]interface{}{
			"code":    0,
			"message": "success",
		},
	}); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送认证响应失败: ServerID=%s | Error=%v", serverID, err)
	} else {
		logger.Debugf("[WebSocket] ✅ 认证响应已发送: ServerID=%s", serverID)
	}
}

func (mc *MiddleConnection) handleHeartbeat(msg *WSMessage) {
	// 响应心跳
	mc.sendMessage(&WSMessage{
		Type: "heartbeat_pong",
		Data: msg.Data,
	})
	logger.Debug("收到心跳，已响应")
}

func (mc *MiddleConnection) forwardToUser(msg *WSMessage) {
	if msg.UserID == 0 {
		return
	}

	userConnMu.RLock()
	userConn, exists := userConnections[msg.UserID]
	userConnMu.RUnlock()

	if !exists {
		logger.Debugf("用户连接不存在: UserID=%d", msg.UserID)
		return
	}

	// 转发消息给用户
	userConn.sendMessage(msg)
}

// handleQueryResponse 处理查询响应消息，发送到响应通道
func (mc *MiddleConnection) handleQueryResponse(msg *WSMessage) {
	if msg.RequestID == "" {
		logger.Warnf("[WebSocket] 查询响应缺少RequestID: Type=%s", msg.Type)
		return
	}

	logger.Debugf("[WebSocket] 处理查询响应: RequestID=%s | Type=%s", msg.RequestID, msg.Type)

	responseChMu.RLock()
	ch, exists := responseChannels[msg.RequestID]
	responseChMu.RUnlock()

	if exists {
		select {
		case ch <- msg:
			logger.Debugf("[WebSocket] ✅ 响应已发送到通道: RequestID=%s", msg.RequestID)
		default:
			logger.Warnf("[WebSocket] ⚠️ 响应通道已满或已关闭: RequestID=%s", msg.RequestID)
		}
	} else {
		logger.Warnf("[WebSocket] ⚠️ 未找到响应通道: RequestID=%s | 当前通道数=%d", msg.RequestID, len(responseChannels))
		// 如果没有响应通道，尝试转发给用户
		if msg.UserID != 0 {
			logger.Debugf("[WebSocket] 尝试转发响应给用户: RequestID=%s | UserID=%d", msg.RequestID, msg.UserID)
			mc.forwardToUser(msg)
		}
	}
}

func (mc *MiddleConnection) sendMessage(msg *WSMessage) error {
	mc.mu.RLock()
	connected := mc.connected
	conn := mc.conn
	mc.mu.RUnlock()

	if !connected || conn == nil {
		logger.Errorf("[WebSocket] 发送消息失败: 连接未建立 | Type=%s | RequestID=%s", msg.Type, msg.RequestID)
		return fmt.Errorf("连接未建立")
	}

	logger.Debugf("[WebSocket] 发送消息到中台: Type=%s | RequestID=%s | UserID=%d", msg.Type, msg.RequestID, msg.UserID)
	if err := conn.WriteJSON(msg); err != nil {
		logger.Errorf("[WebSocket] 发送消息失败: Type=%s | RequestID=%s | Error=%v", msg.Type, msg.RequestID, err)
		return err
	}
	logger.Debugf("[WebSocket] 消息发送成功: Type=%s | RequestID=%s", msg.Type, msg.RequestID)
	return nil
}

func (mc *MiddleConnection) sendError(errorMsg string) {
	mc.sendMessage(&WSMessage{
		Type: "error",
		Data: map[string]interface{}{
			"error": errorMsg,
		},
	})
}

// SendTaskToMiddle 发送任务到中台（同步等待响应）
func SendTaskToMiddle(taskID string, userID int64, deviceID string, workflow map[string]interface{}) error {
	logger.Infof("[WebSocket] 📤 开始发送任务到中台 | TaskID=%s | UserID=%d | DeviceID=%s | Workflow节点数=%d",
		taskID, userID, deviceID, len(workflow))
	logger.Debugf("[WebSocket] 📤 开始发送任务到中台 | TaskID=%s | UserID=%d | DeviceID=%s", taskID, userID, deviceID)

	// 检查中台连接
	middleConnMu.RLock()
	connCount := len(middleConnections)
	var middleConn *MiddleConnection
	var serverIDs []string
	for serverID, conn := range middleConnections {
		serverIDs = append(serverIDs, serverID)
		// 检查连接是否仍然有效
		conn.mu.RLock()
		connected := conn.connected
		conn.mu.RUnlock()
		if connected {
			middleConn = conn
			logger.Debugf("[WebSocket] 找到有效中台连接: ServerID=%s", serverID)
			break
		} else {
			logger.Warnf("[WebSocket] 发现无效连接: ServerID=%s | Connected=%v", serverID, connected)
		}
	}
	middleConnMu.RUnlock()

	if middleConn == nil {
		logger.Errorf("[WebSocket] ❌ 发送任务失败: 没有可用的中台连接 | 当前连接数=%d | ServerIDs=%v | TaskID=%s | UserID=%d",
			connCount, serverIDs, taskID, userID)
		return fmt.Errorf("没有可用的中台连接（连接数=%d，ServerIDs=%v）", connCount, serverIDs)
	}

	// 创建响应通道（使用 taskID 作为 requestID）
	responseCh := make(chan *WSMessage, 1)
	responseChMu.Lock()
	responseChannels[taskID] = responseCh
	responseChMu.Unlock()
	logger.Debugf("[WebSocket] 已创建响应通道: RequestID=%s", taskID)

	// 确保清理响应通道
	defer func() {
		responseChMu.Lock()
		delete(responseChannels, taskID)
		close(responseCh)
		responseChMu.Unlock()
		logger.Debugf("[WebSocket] 已清理响应通道: RequestID=%s", taskID)
	}()

	// 构建任务提交消息
	msg := &WSMessage{
		Type:      "task_submit",
		RequestID: taskID,
		UserID:    userID,
		Data: map[string]interface{}{
			"task_id":   taskID,
			"user_id":   userID,
			"device_id": deviceID,
			"workflow":  workflow,
		},
	}

	// 记录发送到中台的完整消息结构
	msgJSON, _ := json.Marshal(msg)
	logger.Infof("[WebSocket] 📨 准备发送任务提交消息: Type=%s | RequestID=%s | TaskID=%s | UserID=%d | DeviceID=%s",
		msg.Type, msg.RequestID, taskID, userID, deviceID)
	logger.Debugf("[WebSocket] 📨 发送消息结构: RequestID=%s | 消息JSON长度=%d字节", taskID, len(msgJSON))
	logger.Debugf("[WebSocket] 📨 发送消息详情: RequestID=%s | Type=%s | UserID=%d | DeviceID=%s | Workflow节点数=%d",
		taskID, msg.Type, userID, deviceID, len(workflow))

	// 检查连接状态
	middleConn.mu.RLock()
	connected := middleConn.connected
	middleConn.mu.RUnlock()

	if !connected {
		logger.Errorf("[WebSocket] ❌ 中台连接未建立")
		return fmt.Errorf("中台连接未建立")
	}

	// 发送任务提交请求
	if err := middleConn.sendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送任务提交请求失败: RequestID=%s | Error=%v", taskID, err)
		return fmt.Errorf("发送任务提交请求失败: %w", err)
	}

	logger.Debugf("[WebSocket] ✅ 任务提交请求已发送，等待响应: RequestID=%s", taskID)

	// 等待响应（10秒超时，因为任务提交可能需要一些时间）
	select {
	case response := <-responseCh:
		// 记录中台返回的响应结构
		responseJSON, _ := json.Marshal(response)
		logger.Infof("[WebSocket] 📥 收到中台响应: RequestID=%s | Type=%s | UserID=%d",
			taskID, response.Type, response.UserID)
		logger.Debugf("[WebSocket] 📥 响应消息结构: RequestID=%s | 响应JSON长度=%d字节", taskID, len(responseJSON))

		if response.Type == "error" {
			errorMsg := "未知错误"
			var errorDetails map[string]interface{}
			if data, ok := response.Data.(map[string]interface{}); ok {
				errorDetails = data
				if err, ok := data["error"].(string); ok {
					errorMsg = err
				}
			}
			logger.Errorf("[WebSocket] ❌ 收到错误响应: RequestID=%s | Error=%s | 错误详情=%+v",
				taskID, errorMsg, errorDetails)
			return fmt.Errorf(errorMsg)
		}

		// 检查是否是 task_submitted 响应
		if response.Type == "task_submitted" {
			logger.Infof("[WebSocket] ✅ 任务已成功提交到中台: RequestID=%s", taskID)
			if data, ok := response.Data.(map[string]interface{}); ok {
				status, _ := data["status"].(string)
				message, _ := data["message"].(string)
				logger.Debugf("[WebSocket] 📥 中台返回详情: RequestID=%s | Status=%s | Message=%s | 数据字段数=%d",
					taskID, status, message, len(data))
				logger.Debugf("[WebSocket] 📥 中台返回完整数据: RequestID=%s | Data=%+v", taskID, data)
			}
			return nil
		}

		// 其他响应类型视为错误
		logger.Errorf("[WebSocket] ❌ 收到意外的响应类型: RequestID=%s | Type=%s | Response=%+v",
			taskID, response.Type, response)
		return fmt.Errorf("收到意外的响应类型: %s", response.Type)

	case <-time.After(10 * time.Second):
		logger.Errorf("[WebSocket] ❌ 任务提交超时: RequestID=%s | 等待时间=10秒", taskID)
		return fmt.Errorf("任务提交超时")
	}
}

// QueryWorkflowList 查询工作流列表（同步等待响应）
func QueryWorkflowList(userID int64, page, pageSize int, category, search string) (map[string]interface{}, error) {
	logger.Infof("[WebSocket] 📤 开始查询工作流列表 | UserID=%d | Page=%d | PageSize=%d | Category=%s | Search=%s",
		userID, page, pageSize, category, search)

	// 检查中台连接
	middleConnMu.RLock()
	connCount := len(middleConnections)
	var middleConn *MiddleConnection
	var serverIDs []string
	for serverID, conn := range middleConnections {
		serverIDs = append(serverIDs, serverID)
		// 检查连接是否仍然有效
		conn.mu.RLock()
		connected := conn.connected
		conn.mu.RUnlock()
		if connected {
			middleConn = conn
			logger.Debugf("[WebSocket] 找到有效中台连接: ServerID=%s", serverID)
			break
		} else {
			logger.Warnf("[WebSocket] 发现无效连接: ServerID=%s | Connected=%v", serverID, connected)
		}
	}
	middleConnMu.RUnlock()

	if middleConn == nil {
		logger.Errorf("[WebSocket] ❌ 没有可用的中台连接 | 当前连接数=%d | ServerIDs=%v", connCount, serverIDs)
		logger.Errorf("[WebSocket] 💡 提示: 中台连接需要先发送认证消息（auth）才能使用")
		return nil, fmt.Errorf("没有可用的中台连接（连接数=%d，ServerIDs=%v）", connCount, serverIDs)
	}

	logger.Infof("[WebSocket] ✅ 找到中台连接，准备发送查询请求")

	// 生成请求ID
	requestID := fmt.Sprintf("workflow_query_%d_%d", userID, time.Now().UnixNano())
	logger.Debugf("[WebSocket] 生成请求ID: %s", requestID)

	// 创建响应通道
	responseCh := make(chan *WSMessage, 1)
	responseChMu.Lock()
	responseChannels[requestID] = responseCh
	responseChMu.Unlock()
	logger.Debugf("[WebSocket] 已创建响应通道: RequestID=%s", requestID)

	// 确保清理响应通道
	defer func() {
		responseChMu.Lock()
		delete(responseChannels, requestID)
		close(responseCh)
		responseChMu.Unlock()
		logger.Debugf("[WebSocket] 已清理响应通道: RequestID=%s", requestID)
	}()

	// 构建查询消息
	msg := &WSMessage{
		Type:      "workflow_list_query",
		RequestID: requestID,
		UserID:    userID,
		Data: map[string]interface{}{
			"page":      page,
			"page_size": pageSize,
			"category":  category,
			"search":    search,
		},
	}

	logger.Infof("[WebSocket] 📨 准备发送查询消息: Type=%s | RequestID=%s | UserID=%d | Data=%+v",
		msg.Type, msg.RequestID, msg.UserID, msg.Data)

	// 检查连接状态
	middleConn.mu.RLock()
	connected := middleConn.connected
	middleConn.mu.RUnlock()

	if !connected {
		logger.Errorf("[WebSocket] ❌ 中台连接未建立")
		return nil, fmt.Errorf("中台连接未建立")
	}

	// 发送查询请求
	if err := middleConn.sendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送查询请求失败: RequestID=%s | Error=%v", requestID, err)
		return nil, fmt.Errorf("发送查询请求失败: %w", err)
	}

	logger.Infof("[WebSocket] ✅ 查询请求已发送，等待响应: RequestID=%s", requestID)

	// 等待响应（5秒超时）
	select {
	case response := <-responseCh:
		logger.Infof("[WebSocket] 📥 收到响应: RequestID=%s | Type=%s", requestID, response.Type)

		if response.Type == "error" {
			errorMsg := "未知错误"
			if data, ok := response.Data.(map[string]interface{}); ok {
				if err, ok := data["error"].(string); ok {
					errorMsg = err
				}
			}
			logger.Errorf("[WebSocket] ❌ 收到错误响应: RequestID=%s | Error=%s", requestID, errorMsg)
			return nil, fmt.Errorf(errorMsg)
		}

		// 返回响应数据
		if data, ok := response.Data.(map[string]interface{}); ok {
			logger.Infof("[WebSocket] ✅ 查询成功: RequestID=%s | Data=%+v", requestID, data)
			return data, nil
		}
		logger.Errorf("[WebSocket] ❌ 响应数据格式错误: RequestID=%s | Data=%+v", requestID, response.Data)
		return nil, fmt.Errorf("响应数据格式错误")

	case <-time.After(5 * time.Second):
		logger.Errorf("[WebSocket] ❌ 查询超时: RequestID=%s | 等待时间=5秒", requestID)
		return nil, fmt.Errorf("查询超时")
	}
}

// QueryImageFile 查询图像文件（同步等待响应）
// 支持查询单个文件或目录列表
func QueryImageFile(userID int64, taskID string, directory string, filename string) (map[string]interface{}, error) {
	logger.Debugf("[WebSocket] 📤 开始查询图像文件 | UserID=%d | TaskID=%s | Directory=%s | Filename=%s",
		userID, taskID, directory, filename)

	// 检查中台连接
	middleConnMu.RLock()
	connCount := len(middleConnections)
	var middleConn *MiddleConnection
	var serverIDs []string
	for serverID, conn := range middleConnections {
		serverIDs = append(serverIDs, serverID)
		conn.mu.RLock()
		connected := conn.connected
		conn.mu.RUnlock()
		if connected {
			middleConn = conn
			logger.Debugf("[WebSocket] 找到有效中台连接: ServerID=%s", serverID)
			break
		}
	}
	middleConnMu.RUnlock()

	if middleConn == nil {
		logger.Errorf("[WebSocket] ❌ 没有可用的中台连接 | 当前连接数=%d | ServerIDs=%v", connCount, serverIDs)
		return nil, fmt.Errorf("没有可用的中台连接（连接数=%d，ServerIDs=%v）", connCount, serverIDs)
	}

	// 生成请求ID
	requestID := fmt.Sprintf("image_file_query_%d_%d", userID, time.Now().UnixNano())
	logger.Debugf("[WebSocket] 生成请求ID: %s", requestID)

	// 创建响应通道
	responseCh := make(chan *WSMessage, 1)
	responseChMu.Lock()
	responseChannels[requestID] = responseCh
	responseChMu.Unlock()
	logger.Debugf("[WebSocket] 已创建响应通道: RequestID=%s", requestID)

	// 确保清理响应通道
	defer func() {
		responseChMu.Lock()
		delete(responseChannels, requestID)
		close(responseCh)
		responseChMu.Unlock()
		logger.Debugf("[WebSocket] 已清理响应通道: RequestID=%s", requestID)
	}()

	// 构建查询消息
	msgData := map[string]interface{}{
		"task_id": taskID,
		"user_id": float64(userID),
	}
	if directory != "" {
		msgData["directory"] = directory
	}
	if filename != "" {
		msgData["filename"] = filename
	}

	msg := &WSMessage{
		Type:      "image_file_query",
		RequestID: requestID,
		UserID:    userID,
		Data:      msgData,
	}

	logger.Debugf("[WebSocket] 📨 准备发送图像文件查询消息: Type=%s | RequestID=%s | TaskID=%s | Directory=%s | Filename=%s",
		msg.Type, msg.RequestID, taskID, directory, filename)

	// 检查连接状态
	middleConn.mu.RLock()
	connected := middleConn.connected
	middleConn.mu.RUnlock()

	if !connected {
		logger.Errorf("[WebSocket] ❌ 中台连接未建立")
		return nil, fmt.Errorf("中台连接未建立")
	}

	// 发送查询请求
	if err := middleConn.sendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送查询请求失败: RequestID=%s | Error=%v", requestID, err)
		return nil, fmt.Errorf("发送查询请求失败: %w", err)
	}

	logger.Debugf("[WebSocket] ✅ 查询请求已发送，等待响应: RequestID=%s", requestID)

	// 等待响应（5秒超时）
	select {
	case response := <-responseCh:
		logger.Debugf("[WebSocket] 📥 收到响应: RequestID=%s | Type=%s", requestID, response.Type)

		if response.Type == "error" {
			errorMsg := "未知错误"
			if data, ok := response.Data.(map[string]interface{}); ok {
				if err, ok := data["error"].(string); ok {
					errorMsg = err
				}
			}
			logger.Errorf("[WebSocket] ❌ 收到错误响应: RequestID=%s | Error=%s", requestID, errorMsg)
			return nil, fmt.Errorf(errorMsg)
		}

		// 返回响应数据
		if data, ok := response.Data.(map[string]interface{}); ok {
			logger.Debugf("[WebSocket] ✅ 查询成功: RequestID=%s | TaskID=%s", requestID, taskID)
			return data, nil
		}
		logger.Errorf("[WebSocket] ❌ 响应数据格式错误: RequestID=%s | Data=%+v", requestID, response.Data)
		return nil, fmt.Errorf("响应数据格式错误")

	case <-time.After(5 * time.Second):
		logger.Errorf("[WebSocket] ❌ 查询超时: RequestID=%s | 等待时间=5秒", requestID)
		return nil, fmt.Errorf("查询超时")
	}
}

// QueryTaskStatus 查询任务状态（同步等待响应）
func QueryTaskStatus(userID int64, taskID string) (map[string]interface{}, error) {
	logger.Debugf("[WebSocket] 📤 开始查询任务状态 | UserID=%d | TaskID=%s", userID, taskID)

	// 检查中台连接
	middleConnMu.RLock()
	connCount := len(middleConnections)
	var middleConn *MiddleConnection
	var serverIDs []string
	for serverID, conn := range middleConnections {
		serverIDs = append(serverIDs, serverID)
		// 检查连接是否仍然有效
		conn.mu.RLock()
		connected := conn.connected
		conn.mu.RUnlock()
		if connected {
			middleConn = conn
			logger.Debugf("[WebSocket] 找到有效中台连接: ServerID=%s", serverID)
			break
		} else {
			logger.Warnf("[WebSocket] 发现无效连接: ServerID=%s | Connected=%v", serverID, connected)
		}
	}
	middleConnMu.RUnlock()

	if middleConn == nil {
		logger.Errorf("[WebSocket] ❌ 没有可用的中台连接 | 当前连接数=%d | ServerIDs=%v", connCount, serverIDs)
		return nil, fmt.Errorf("没有可用的中台连接（连接数=%d，ServerIDs=%v）", connCount, serverIDs)
	}

	// 生成请求ID
	requestID := fmt.Sprintf("task_status_query_%d_%d", userID, time.Now().UnixNano())
	logger.Debugf("[WebSocket] 生成请求ID: %s", requestID)

	// 创建响应通道
	responseCh := make(chan *WSMessage, 1)
	responseChMu.Lock()
	responseChannels[requestID] = responseCh
	responseChMu.Unlock()
	logger.Debugf("[WebSocket] 已创建响应通道: RequestID=%s", requestID)

	// 确保清理响应通道
	defer func() {
		responseChMu.Lock()
		delete(responseChannels, requestID)
		close(responseCh)
		responseChMu.Unlock()
		logger.Debugf("[WebSocket] 已清理响应通道: RequestID=%s", requestID)
	}()

	// 构建查询消息
	msg := &WSMessage{
		Type:      "task_status_query",
		RequestID: requestID,
		UserID:    userID,
		Data: map[string]interface{}{
			"task_id": taskID,
			"user_id": float64(userID),
		},
	}

	logger.Debugf("[WebSocket] 📨 准备发送任务状态查询消息: Type=%s | RequestID=%s | TaskID=%s",
		msg.Type, msg.RequestID, taskID)

	// 检查连接状态
	middleConn.mu.RLock()
	connected := middleConn.connected
	middleConn.mu.RUnlock()

	if !connected {
		logger.Errorf("[WebSocket] ❌ 中台连接未建立")
		return nil, fmt.Errorf("中台连接未建立")
	}

	// 发送查询请求
	if err := middleConn.sendMessage(msg); err != nil {
		logger.Errorf("[WebSocket] ❌ 发送查询请求失败: RequestID=%s | Error=%v", requestID, err)
		return nil, fmt.Errorf("发送查询请求失败: %w", err)
	}

	logger.Debugf("[WebSocket] ✅ 查询请求已发送，等待响应: RequestID=%s", requestID)

	// 等待响应（5秒超时）
	select {
	case response := <-responseCh:
		logger.Debugf("[WebSocket] 📥 收到响应: RequestID=%s | Type=%s", requestID, response.Type)

		if response.Type == "error" {
			errorMsg := "未知错误"
			if data, ok := response.Data.(map[string]interface{}); ok {
				if err, ok := data["error"].(string); ok {
					errorMsg = err
				}
			}
			logger.Errorf("[WebSocket] ❌ 收到错误响应: RequestID=%s | Error=%s", requestID, errorMsg)
			return nil, fmt.Errorf(errorMsg)
		}

		// 返回响应数据
		if data, ok := response.Data.(map[string]interface{}); ok {
			logger.Debugf("[WebSocket] ✅ 查询成功: RequestID=%s | Status=%v | Progress=%v",
				requestID, data["status"], data["progress"])
			return data, nil
		}
		logger.Errorf("[WebSocket] ❌ 响应数据格式错误: RequestID=%s | Data=%+v", requestID, response.Data)
		return nil, fmt.Errorf("响应数据格式错误")

	case <-time.After(5 * time.Second):
		logger.Errorf("[WebSocket] ❌ 查询超时: RequestID=%s | 等待时间=5秒", requestID)
		return nil, fmt.Errorf("查询超时")
	}
}

// WSMessage WebSocket消息结构
type WSMessage struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id,omitempty"`
	UserID    int64       `json:"user_id,omitempty"`
	Data      interface{} `json:"data"`
}

// HandleUserWebSocket 处理用户WebSocket连接（用于实时推送）
func HandleUserWebSocket(c *gin.Context) {
	userID := c.MustGet("user_id").(int64)

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorf("WebSocket升级失败: %v", err)
		return
	}

	userConn := &UserConnection{
		conn:      conn,
		userID:    userID,
		connected: true,
	}

	// 添加到用户连接池
	userConnMu.Lock()
	userConnections[userID] = userConn
	userConnMu.Unlock()

	logger.Infof("用户WebSocket连接已建立: UserID=%d", userID)

	// 启动消息处理（处理中台转发的消息）
	go userConn.handleMessages()

	// 启动心跳
	go userConn.startHeartbeat()
}

func (uc *UserConnection) handleMessages() {
	defer func() {
		// 捕获 panic，防止程序崩溃
		if r := recover(); r != nil {
			logger.Errorf("[WebSocket] 用户WebSocket消息监听发生panic: %v", r)
		}

		uc.mu.Lock()
		uc.connected = false
		conn := uc.conn
		uc.mu.Unlock()

		if conn != nil {
			conn.Close()
		}

		// 从连接池移除
		userConnMu.Lock()
		delete(userConnections, uc.userID)
		userConnMu.Unlock()
	}()

	for {
		uc.mu.RLock()
		conn := uc.conn
		connected := uc.connected
		uc.mu.RUnlock()

		if conn == nil || !connected {
			return
		}

		// 设置读取超时（60秒，用于心跳检测）
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[WebSocket] 设置读取超时失败: %v", err)
			return
		}

		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			// 检查是否是超时错误（这是唯一可以继续的错误）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				// 超时是正常的，继续循环等待
				logger.Debugf("[WebSocket] 读取超时，继续等待消息")
				continue
			}

			// 检查是否是正常的关闭错误
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 这是意外的关闭错误
				logger.Warnf("[WebSocket] WebSocket连接意外关闭: %v", err)
			} else {
				// 其他所有错误（包括正常关闭、EOF、连接重置等）都应该退出
				logger.Warnf("[WebSocket] 读取消息失败，连接已断开: %v", err)
			}
			// 无论什么错误，都退出循环，避免重复读取失败的连接
			return
		}

		// 收到消息后重置超时
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("[WebSocket] 重置读取超时失败: %v", err)
			return
		}

		uc.handleMessage(&msg)
	}
}

func (uc *UserConnection) handleMessage(msg *WSMessage) {
	switch msg.Type {
	case "task_progress":
		uc.handleTaskProgress(msg)
	case "task_complete", "task_failed":
		uc.handleTaskComplete(msg)
	case "image_ready":
		uc.handleImageReady(msg)
	case "task_status_query":
		uc.handleTaskStatusQuery(msg)
	case "image_query":
		uc.handleImageQuery(msg)
	case "queue_query":
		uc.handleQueueQuery(msg)
	}
}

func (uc *UserConnection) handleTaskProgress(msg *WSMessage) {
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return
	}

	taskID, _ := data["task_id"].(string)
	progress, _ := data["progress"].(float64)
	status, _ := data["status"].(string)

	// 调用回调函数更新任务状态
	if taskUpdateCallback != nil {
		taskUpdateCallback(taskID, status, int(progress), "", "")
	}
}

func (uc *UserConnection) handleTaskComplete(msg *WSMessage) {
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return
	}

	taskID, _ := data["task_id"].(string)
	promptID, _ := data["prompt_id"].(string)
	status, _ := data["status"].(string)
	errorMsg, _ := data["error"].(string)

	// 调用回调函数更新任务状态
	if taskUpdateCallback != nil {
		taskUpdateCallback(taskID, status, 100, promptID, errorMsg)
	}

	// 处理图像列表
	if fileAddCallback != nil {
		if images, ok := data["images"].([]interface{}); ok {
			for _, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					fileAddCallback(taskID, uc.userID, imgMap)
				}
			}
		}
	}
}

func (uc *UserConnection) handleImageReady(msg *WSMessage) {
	data, ok := msg.Data.(map[string]interface{})
	if !ok {
		return
	}

	taskID, _ := data["task_id"].(string)
	fileInfo, ok := data["file"].(map[string]interface{})
	if !ok {
		return
	}

	// 调用回调函数添加文件记录
	if fileAddCallback != nil {
		fileAddCallback(taskID, uc.userID, fileInfo)
	}
}

func (uc *UserConnection) handleTaskStatusQuery(msg *WSMessage) {
	// 用户查询任务状态，转发到中台
	middleConnMu.RLock()
	var middleConn *MiddleConnection
	for _, conn := range middleConnections {
		middleConn = conn
		break
	}
	middleConnMu.RUnlock()

	if middleConn != nil {
		middleConn.sendMessage(msg)
	}
}

func (uc *UserConnection) handleImageQuery(msg *WSMessage) {
	// 用户查询图像，转发到中台
	middleConnMu.RLock()
	var middleConn *MiddleConnection
	for _, conn := range middleConnections {
		middleConn = conn
		break
	}
	middleConnMu.RUnlock()

	if middleConn != nil {
		middleConn.sendMessage(msg)
	}
}

func (uc *UserConnection) handleQueueQuery(msg *WSMessage) {
	// 用户查询队列，转发到中台
	middleConnMu.RLock()
	var middleConn *MiddleConnection
	for _, conn := range middleConnections {
		middleConn = conn
		break
	}
	middleConnMu.RUnlock()

	if middleConn != nil {
		middleConn.sendMessage(msg)
	}
}

func (uc *UserConnection) sendMessage(msg *WSMessage) error {
	uc.mu.RLock()
	defer uc.mu.RUnlock()

	if !uc.connected || uc.conn == nil {
		return fmt.Errorf("用户连接未建立")
	}

	return uc.conn.WriteJSON(msg)
}

func (uc *UserConnection) startHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		uc.mu.RLock()
		connected := uc.connected
		uc.mu.RUnlock()

		if !connected {
			return
		}

		// 发送心跳
		uc.sendMessage(&WSMessage{
			Type: "heartbeat_ping",
			Data: map[string]interface{}{
				"timestamp": time.Now().Unix(),
			},
		})
	}
}
