package websocket

import (
	"comfyui_middleground/logger"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"comfyui_middleground/config"

	"github.com/gorilla/websocket"
)

type FrontWSClient struct {
	conn      *websocket.Conn
	config    *config.FrontServerConfig
	connected bool
	mu        sync.RWMutex
	handlers  map[string]func(map[string]interface{})
}

type WSMessage struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id,omitempty"`
	UserID    int64       `json:"user_id,omitempty"`
	Data      interface{} `json:"data"`
}

func NewFrontWSClient(cfg *config.FrontServerConfig) *FrontWSClient {
	return &FrontWSClient{
		config:   cfg,
		handlers: make(map[string]func(map[string]interface{})),
	}
}

func (c *FrontWSClient) Connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 100000 * time.Second,
	}

	conn, _, err := dialer.Dial(c.config.WSURL, nil)
	if err != nil {
		return fmt.Errorf("连接前台WebSocket失败: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	logger.Infof("已连接到前台WebSocket: %s", c.config.WSURL)

	// 发送认证消息
	if err := c.sendAuth(); err != nil {
		c.mu.Lock()
		c.connected = false
		c.conn = nil
		c.mu.Unlock()
		conn.Close()
		return fmt.Errorf("发送认证失败: %w", err)
	}

	// 启动心跳
	go c.startHeartbeat()

	// 启动消息监听
	go c.listenMessages()

	return nil
}

// Start 在后台持续尝试连接，连接失败不影响调用者
func (c *FrontWSClient) Start() {
	go c.connectLoop()
}

// connectLoop 持续尝试连接，直到成功
func (c *FrontWSClient) connectLoop() {
	for {
		if err := c.Connect(); err != nil {
			logger.Warnf("连接前台WebSocket失败: %v，%v后重试", err, c.config.ReconnectInterval)
			time.Sleep(c.config.ReconnectInterval)
			continue
		}
		// 连接成功，等待连接断开
		// listenMessages会在连接断开时设置connected=false，然后这里会检测到并重连
		for {
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()
			if !connected {
				logger.Infof("检测到连接断开，%v后重连", c.config.ReconnectInterval)
				time.Sleep(c.config.ReconnectInterval)
				break
			}
			time.Sleep(1 * time.Second) // 每秒检查一次连接状态
		}
	}
}

func (c *FrontWSClient) sendAuth() error {
	authMsg := WSMessage{
		Type: "auth",
		Data: map[string]interface{}{
			"server_id":  c.config.ServerID,
			"secret_key": c.config.SecretKey,
			"version":    "1.0.0",
		},
	}
	return c.SendMessage(authMsg)
}

func (c *FrontWSClient) SendMessage(msg interface{}) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("未连接到前台WebSocket")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *FrontWSClient) startHeartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()

			if connected {
				msg := WSMessage{
					Type: "heartbeat",
					Data: map[string]interface{}{
						"timestamp": time.Now().Unix(),
					},
				}
				if err := c.SendMessage(msg); err != nil {
					logger.Warnf("发送心跳失败: %v", err)
				}
			} else {
				return
			}
		}
	}
}

func (c *FrontWSClient) listenMessages() {
	defer func() {
		// 捕获 panic，防止程序崩溃
		if r := recover(); r != nil {
			logger.Errorf("WebSocket消息监听发生panic: %v", r)
		}

		c.mu.Lock()
		c.connected = false
		if c.conn != nil {
			// 尝试关闭连接，忽略错误
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
		logger.Debugf("前台WebSocket消息监听已退出")
	}()

	for {
		c.mu.RLock()
		conn := c.conn
		connected := c.connected
		c.mu.RUnlock()

		if conn == nil || !connected {
			return
		}

		// 设置读取超时（60秒，用于心跳检测）
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("设置读取超时失败: %v", err)
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			// 检查是否是超时错误（这是唯一可以继续的错误）
			if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
				// 超时是正常的，继续循环等待
				logger.Debugf("读取超时，继续等待消息")
				continue
			}

			// 检查是否是正常的关闭错误
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// 这是意外的关闭错误，应该退出
				logger.Warnf("WebSocket连接意外关闭: %v", err)
			} else {
				// 其他所有错误（包括正常关闭、EOF、连接重置等）都应该退出
				logger.Warnf("读取消息失败，连接已断开: %v", err)
			}
			// 无论什么错误，都退出循环，避免重复读取失败的连接
			return
		}

		// 收到消息后重置超时
		if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			logger.Warnf("重置读取超时失败: %v", err)
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			logger.Warnf("解析消息失败: %v", err)
			continue
		}

		logger.Debugf("[WebSocket] 收到消息: Type=%s, RequestID=%s, UserID=%d", msg.Type, msg.RequestID, msg.UserID)

		// 处理消息
		if handler, ok := c.handlers[msg.Type]; ok {
			// 将 RequestID 和 UserID 合并到 Data 中，以便处理器可以访问
			data := make(map[string]interface{})
			if msgData, ok := msg.Data.(map[string]interface{}); ok {
				// 复制 Data 中的字段
				for k, v := range msgData {
					data[k] = v
				}
			}
			// 添加顶层字段到 data（如果 data 中没有这些字段）
			if msg.RequestID != "" {
				if _, exists := data["request_id"]; !exists {
					data["request_id"] = msg.RequestID
				}
			}
			if msg.UserID != 0 {
				if _, exists := data["user_id"]; !exists {
					data["user_id"] = float64(msg.UserID) // 转换为 float64 以匹配 JSON 解析后的类型
				}
			}
			logger.Debugf("[WebSocket] 调用处理器: Type=%s, Data=%+v", msg.Type, data)
			go handler(data)
		} else {
			logger.Warnf("[WebSocket] 未找到消息处理器: Type=%s", msg.Type)
		}
	}
}

func (c *FrontWSClient) RegisterHandler(msgType string, handler func(map[string]interface{})) {
	c.handlers[msgType] = handler
}

func (c *FrontWSClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *FrontWSClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
