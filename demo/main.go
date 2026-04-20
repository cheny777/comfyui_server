package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	ComfyUIHost = "192.168.31.14:8189"
	ComfyUIURL  = "ws://192.168.31.14:8189/ws?clientId="
)

// ComfyUI 消息结构
type ComfyUIMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// 工作流节点
type Node struct {
	ClassType string                 `json:"class_type"`
	Inputs    map[string]interface{} `json:"inputs"`
}

// 工作流
type Workflow map[string]Node

// 客户端
type ComfyUIClient struct {
	conn   *websocket.Conn
	client *http.Client
	host   string
}

// 创建新的 ComfyUI 客户端
func NewComfyUIClient(host string) *ComfyUIClient {
	return &ComfyUIClient{
		client: &http.Client{Timeout: 30 * time.Second},
		host:   host,
	}
}

// 连接到 WebSocket
func (c *ComfyUIClient) Connect() error {
	clientID := fmt.Sprintf("go-client-%d", time.Now().Unix())
	url := fmt.Sprintf("ws://%s/ws?clientId=%s", c.host, clientID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	c.conn = conn
	log.Printf("已连接到 ComfyUI: %s", url)
	return nil
}

// 关闭连接
func (c *ComfyUIClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// 从 JSON 文件加载工作流
func loadWorkflowFromFile(filename string) (Workflow, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	var workflow Workflow
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&workflow); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	// 清理工作流，移除 _meta 字段（ComfyUI API 不需要）
	cleanedWorkflow := make(Workflow)
	for key, node := range workflow {
		cleanedNode := Node{
			ClassType: node.ClassType,
			Inputs:    node.Inputs,
		}
		cleanedWorkflow[key] = cleanedNode
	}

	log.Printf("✅ 成功加载工作流，包含 %d 个节点", len(cleanedWorkflow))
	return cleanedWorkflow, nil
}

// 提交工作流
func (c *ComfyUIClient) QueuePrompt(workflow Workflow) (string, error) {
	// 构建提示数据
	promptData := map[string]interface{}{
		"prompt":    workflow,
		"client_id": fmt.Sprintf("go-client-%d", time.Now().Unix()),
	}

	// 发送队列提示请求
	url := fmt.Sprintf("http://%s/prompt", c.host)
	jsonData, err := json.Marshal(promptData)
	if err != nil {
		return "", fmt.Errorf("序列化失败: %w", err)
	}

	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
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

	log.Printf("📤 任务已提交，Prompt ID: %s", promptID)
	return promptID, nil
}

// 监听 WebSocket 消息
func (c *ComfyUIClient) ListenForImages(promptID string, outputDir string, workflow Workflow) error {
	if c.conn == nil {
		return fmt.Errorf("未连接到 WebSocket")
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📡 开始监听图像生成 (Prompt ID: %s)", promptID)
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 查找 SaveImage 节点 ID
	saveImageNodeIDs := make(map[int]bool)
	for nodeID, node := range workflow {
		if node.ClassType == "SaveImage" {
			var id int
			fmt.Sscanf(nodeID, "%d", &id)
			saveImageNodeIDs[id] = true
			log.Printf("✓ 找到 SaveImage 节点: %s", nodeID)
		}
	}

	imageDownloaded := false
	lastProgress := -1
	executedNodes := make(map[int]bool)

	for {
		messageType, message, err := c.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("读取消息失败: %w", err)
		}

		// 只处理文本消息，忽略二进制消息
		if messageType != websocket.TextMessage {
			continue
		}

		// 过滤掉空消息和包含空字符的消息
		messageStr := strings.TrimSpace(string(message))
		if len(messageStr) == 0 || strings.Contains(messageStr, "\x00") {
			continue
		}

		// 尝试解析 JSON
		var msg ComfyUIMessage
		if err := json.Unmarshal([]byte(messageStr), &msg); err != nil {
			// 静默忽略无效的 JSON 消息（可能是其他类型的消息）
			continue
		}

		switch msg.Type {
		case "execution_cached":
			log.Println("⚡ 执行已缓存（使用缓存结果）")
		case "execution_start":
			log.Println("🚀 开始执行工作流")
		case "execution_success":
			log.Println("✅ 执行成功完成")
			// 等待一段时间确保所有消息都处理完
			time.Sleep(2 * time.Second)
			if imageDownloaded {
				log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				log.Println("🎉 所有图像已下载完成")
				log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				return nil
			} else {
				log.Println("⚠️  执行成功但未检测到图像下载，尝试查询历史记录...")
				if c.queryHistoryAndDownload(promptID, outputDir) {
					log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
					log.Println("🎉 从历史记录成功下载图像")
					log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				} else {
					log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
					log.Println("⚠️  未找到可下载的图像")
					log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				}
				return nil
			}
		case "progress":
			if data, ok := msg.Data.(map[string]interface{}); ok {
				if value, ok := data["value"].(float64); ok {
					if max, ok := data["max"].(float64); ok {
						progress := int((value / max) * 100)
						// 只在进度变化时打印，减少日志噪音
						if progress != lastProgress && progress%5 == 0 {
							log.Printf("📊 生成进度: %d%%", progress)
							lastProgress = progress
						}
					}
				}
			}
		case "executed":
			if data, ok := msg.Data.(map[string]interface{}); ok {
				nodeIDFloat, ok := data["node"].(float64)
				if !ok {
					continue
				}
				nodeID := int(nodeIDFloat)
				executedNodes[nodeID] = true

				// 检查是否是 SaveImage 节点
				if saveImageNodeIDs[nodeID] {
					log.Printf("🖼️  SaveImage 节点 %d 执行完成", nodeID)
				} else {
					log.Printf("✓ 节点 %d 执行完成", nodeID)
				}

				// 检查是否有图像输出
				if outputs, ok := data["output"].(map[string]interface{}); ok {
					if images, ok := outputs["images"].([]interface{}); ok && len(images) > 0 {
						log.Printf("📥 检测到 %d 张图像需要下载", len(images))
						for i, img := range images {
							if imgMap, ok := img.(map[string]interface{}); ok {
								filename, _ := imgMap["filename"].(string)
								subfolder, _ := imgMap["subfolder"].(string)
								imgType, _ := imgMap["type"].(string)

								if filename != "" {
									log.Printf("  [%d/%d] 下载图像: %s", i+1, len(images), filename)
									if err := c.downloadImage(filename, subfolder, imgType, outputDir); err != nil {
										log.Printf("❌ 下载图像失败: %v", err)
									} else {
										imageDownloaded = true
										log.Printf("✅ 图像下载成功")
									}
								}
							}
						}
					} else {
						// 调试：打印节点输出信息
						if saveImageNodeIDs[nodeID] {
							log.Printf("⚠️  SaveImage 节点 %d 没有图像输出，输出内容: %+v", nodeID, outputs)
						}
					}
				}
			}
		case "execution_error":
			log.Printf("❌ 执行错误: %v", msg.Data)
			return fmt.Errorf("执行错误: %v", msg.Data)
		default:
			// 其他类型的消息，可以在这里添加处理逻辑
		}
	}
}

// 查询历史记录并下载图像
func (c *ComfyUIClient) queryHistoryAndDownload(promptID string, outputDir string) bool {
	url := fmt.Sprintf("http://%s/history", c.host)
	resp, err := c.client.Get(url)
	if err != nil {
		log.Printf("❌ 查询历史记录失败: %v", err)
		return false
	}
	defer resp.Body.Close()

	var history map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&history); err != nil {
		log.Printf("❌ 解析历史记录失败: %v", err)
		return false
	}

	downloaded := false
	// 查找历史记录中对应 promptID 的输出
	if promptData, ok := history[promptID].(map[string]interface{}); ok {
		if outputs, ok := promptData["outputs"].(map[string]interface{}); ok {
			log.Println("🔍 从历史记录中查找图像...")
			for nodeID, nodeOutput := range outputs {
				if outputMap, ok := nodeOutput.(map[string]interface{}); ok {
					if images, ok := outputMap["images"].([]interface{}); ok && len(images) > 0 {
						log.Printf("📥 在节点 %s 找到 %d 张图像", nodeID, len(images))
						for i, img := range images {
							if imgMap, ok := img.(map[string]interface{}); ok {
								filename, _ := imgMap["filename"].(string)
								subfolder, _ := imgMap["subfolder"].(string)
								imgType, _ := imgMap["type"].(string)

								if filename != "" {
									log.Printf("  [%d/%d] 下载图像: %s", i+1, len(images), filename)
									if err := c.downloadImage(filename, subfolder, imgType, outputDir); err != nil {
										log.Printf("❌ 下载图像失败: %v", err)
									} else {
										downloaded = true
										log.Printf("✅ 图像下载成功")
									}
								}
							}
						}
					}
				}
			}
		} else {
			log.Printf("⚠️  历史记录中未找到 outputs 字段")
		}
	} else {
		log.Printf("⚠️  历史记录中未找到 prompt ID: %s", promptID)
	}

	if !downloaded {
		log.Println("⚠️  历史记录中未找到图像")
	}

	return downloaded
}

// 下载图像
func (c *ComfyUIClient) downloadImage(filename, subfolder, imgType, outputDir string) error {
	var url string
	if subfolder != "" {
		url = fmt.Sprintf("http://%s/view?filename=%s&subfolder=%s&type=%s",
			c.host, filename, subfolder, imgType)
	} else {
		url = fmt.Sprintf("http://%s/view?filename=%s&type=%s",
			c.host, filename, imgType)
	}

	resp, err := c.client.Get(url)
	if err != nil {
		return fmt.Errorf("下载图像失败: %w", err)
	}
	defer resp.Body.Close()

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	// 保存文件
	filePath := fmt.Sprintf("%s/%s", outputDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	log.Printf("💾 图像已保存: %s", filePath)
	return nil
}

func main() {
	// 从 JSON 文件加载工作流
	workflowFile := "常用.json"
	workflow, err := loadWorkflowFromFile(workflowFile)
	if err != nil {
		log.Fatalf("加载工作流失败: %v", err)
	}

	// 创建客户端
	client := NewComfyUIClient(ComfyUIHost)
	defer client.Close()

	// 连接到 WebSocket
	if err := client.Connect(); err != nil {
		log.Fatalf("连接失败: %v", err)
	}

	// 提交任务
	promptID, err := client.QueuePrompt(workflow)
	if err != nil {
		log.Fatalf("提交任务失败: %v", err)
	}

	// 监听并下载图像
	outputDir := "./output"
	if err := client.ListenForImages(promptID, outputDir, workflow); err != nil {
		log.Fatalf("监听失败: %v", err)
	}
}
