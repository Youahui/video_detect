package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type DetectionResult struct {
	X1    int     `json:"x1"`
	Y1    int     `json:"y1"`
	X2    int     `json:"x2"`
	Y2    int     `json:"y2"`
	Label string  `json:"label"`
	Conf  float64 `json:"conf"`
}

type DetectResponse struct {
	Success bool              `json:"success"`
	Count   int               `json:"count"`
	Results []DetectionResult `json:"results"`
	TimeMs  int               `json:"time_ms"`
	Error   string            `json:"error"`
}

var bufPool sync.Pool

func init() {
	bufPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
}

// 监测对象
func detectObjects(data []byte, aiURL string) ([]DetectionResult, error) {
	if data == nil || len(data) == 0 {
		return nil, fmt.Errorf("图像识别 输入数据为空")
	}

	// 使用 bufPool 复用内存
	buf := bufPool.Get().(*bytes.Buffer)
	defer bufPool.Put(buf)
	buf.Reset()
	buf.Write(data)

	resp, err := http.Post(aiURL, "application/octet-stream", buf)
	if err != nil {
		return nil, fmt.Errorf("图像识别 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("图像识别 服务错误: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("图像识别 响应读取失败: %w", err)
	}

	var detectResp DetectResponse
	if err := json.Unmarshal(body, &detectResp); err != nil {
		return nil, fmt.Errorf("图像识别 JSON解析失败: %w", err)
	}

	if !detectResp.Success {
		return nil, fmt.Errorf("图像识别 返回标记失败: %s", detectResp.Error)
	}

	return detectResp.Results, nil
}

// detectObjectsUvicronSocket 开启 UvicronSocket 减少 tcp损耗
func detectObjectsUvicronSocket(data []byte, socketPath, aiURL string) ([]DetectionResult, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	defer bufPool.Put(buf)
	buf.Reset()
	buf.Write(data)

	// 创建 Unix Socket HTTP 客户端
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 5 * time.Second, // 超时控制，可调
	}

	req, err := http.NewRequest("POST", aiURL, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("图像识别 服务错误: %d, 响应: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("图像识别 响应读取失败: %w", err)
	}

	var detectResp DetectResponse
	if err := json.Unmarshal(body, &detectResp); err != nil {
		return nil, fmt.Errorf("图像识别 JSON解析失败: %w", err)
	}

	if !detectResp.Success {
		return nil, fmt.Errorf("图像识别 返回标记失败: %s", detectResp.Error)
	}

	return detectResp.Results, nil
}
