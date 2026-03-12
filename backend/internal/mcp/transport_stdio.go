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

func newStdioClient(cfg *ServerConfig) (Client, error) {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return nil, fmt.Errorf("stdio 缺少 command")
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

func (c *stdioClient) initialize(ctx context.Context) error {
	_, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "corner",
			"version": "1.2",
		},
	})
	if err != nil {
		return fmt.Errorf("initialize 失败: %w", err)
	}
	_, err = c.request(ctx, "notifications/initialized", map[string]any{})
	return err
}

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
	if errObj, ok := rpc["error"]; ok && errObj != nil {
		return nil, fmt.Errorf("mcp rpc error: %v", errObj)
	}
	result, _ := rpc["result"].(map[string]any)
	return result, nil
}

func readRPCMessage(ctx context.Context, r io.Reader) ([]byte, error) {
	reader := bufio.NewReader(r)
	_ = ctx
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
				return nil, fmt.Errorf("content-length 非法: %w", err)
			}
			contentLength = n
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("缺少 content-length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *stdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	toolItems, _ := result["tools"].([]any)
	tools := make([]Tool, 0, len(toolItems))
	for _, item := range toolItems {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := obj["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		description, _ := obj["description"].(string)
		inputSchema, _ := obj["inputSchema"].(map[string]any)
		tools = append(tools, Tool{Name: name, Description: description, InputSchema: inputSchema})
	}
	return tools, nil
}

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
