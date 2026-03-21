package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type stdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu sync.Mutex
	id int64
}

// newStdioClient 启动 MCP 子进程并完成初始化握手。
func newStdioClient(cfg *ServerConfig) (Client, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("stdio command is required")
	}

	cmd := exec.Command(command, cfg.Args...)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	client := &stdioClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}
	if err := client.initialize(context.Background()); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *stdioClient) nextID() int64 {
	return atomic.AddInt64(&c.id, 1)
}

// initialize 执行 MCP initialize 流程，并发送 initialized 通知。
func (c *stdioClient) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "slimebot",
			"version": "1.2",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}
	_, err = c.request(ctx, "notifications/initialized", map[string]any{})
	return err
}

// request 通过 stdio 串行发送 JSON-RPC 请求并读取对应响应。
func (c *stdioClient) request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.nextID(),
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(body); err != nil {
		return nil, err
	}

	respRaw, err := readRPCMessage(ctx, c.stdout)
	if err != nil {
		return nil, err
	}
	var rpc map[string]any
	if err := json.Unmarshal(respRaw, &rpc); err != nil {
		return nil, err
	}
	// 与 HTTP 传输保持一致：优先处理 RPC 层错误对象。
	if errObj, ok := rpc["error"]; ok && errObj != nil {
		return nil, fmt.Errorf("mcp rpc error: %v", errObj)
	}
	result, _ := rpc["result"].(map[string]any)
	return result, nil
}

func readRPCMessage(ctx context.Context, r io.Reader) ([]byte, error) {
	type rpcResult struct {
		data []byte
		err  error
	}
	ch := make(chan rpcResult, 1)
	go func() {
		data, err := readRPCMessageBlocking(r)
		ch <- rpcResult{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}

func readRPCMessageBlocking(r io.Reader) ([]byte, error) {
	reader := bufio.NewReader(r)
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			value = strings.TrimSpace(strings.TrimPrefix(value, "content-length:"))
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing content-length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ListTools 获取 MCP 服务工具列表并转为内部统一结构。
func (c *stdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTools(result), nil
}

// CallTool 调用指定工具并返回标准化调用结果。
func (c *stdioClient) CallTool(ctx context.Context, name string, arguments map[string]any) (*CallResult, error) {
	result, err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})
	if err != nil {
		return nil, err
	}
	return parseCallResult(result), nil
}

// Close 关闭 stdio 管道并终止子进程，避免僵尸进程残留。
func (c *stdioClient) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.stdout != nil {
		_ = c.stdout.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	return nil
}
