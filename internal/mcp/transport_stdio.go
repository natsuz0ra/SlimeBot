package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type stdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader

	mu sync.Mutex
	id int64
}

// newStdioClient starts an MCP subprocess and completes the initialize handshake.
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
		reader: bufio.NewReader(stdout),
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

// initialize runs MCP initialize and sends initialized.
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
	return c.notify("notifications/initialized", map[string]any{})
}

// request sends one JSON-RPC over stdio and reads the matching response.
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
	if _, err := c.stdin.Write(body); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(c.stdin, "\n"); err != nil {
		return nil, err
	}

	if c.reader == nil {
		c.reader = bufio.NewReader(c.stdout)
	}
	respRaw, err := readRPCMessage(ctx, c.reader)
	if err != nil {
		return nil, err
	}
	var rpc map[string]any
	if err := json.Unmarshal(respRaw, &rpc); err != nil {
		return nil, err
	}
	// Match HTTP transport: surface RPC-level error objects first.
	if errObj, ok := rpc["error"]; ok && errObj != nil {
		return nil, fmt.Errorf("mcp rpc error: %v", errObj)
	}
	result, _ := rpc["result"].(map[string]any)
	return result, nil
}

// notify sends one JSON-RPC notification over stdio without waiting for a response.
func (c *stdioClient) notify(method string, params map[string]any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(body); err != nil {
		return err
	}
	_, err = io.WriteString(c.stdin, "\n")
	return err
}

func readRPCMessage(ctx context.Context, r io.Reader) ([]byte, error) {
	reader := r.(*bufio.Reader)
	type rpcResult struct {
		data []byte
		err  error
	}
	ch := make(chan rpcResult, 1)
	go func() {
		data, err := readRPCMessageBlocking(reader)
		ch <- rpcResult{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.data, res.err
	}
}

func readRPCMessageBlocking(reader *bufio.Reader) ([]byte, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		return []byte(line), nil
	}
}

// ListTools fetches tools from the MCP server into the internal Tool shape.
func (c *stdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	result, err := c.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	return parseTools(result), nil
}

// CallTool invokes a tool and returns a normalized CallResult.
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

// Close closes stdio and terminates the child process to avoid zombies.
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
