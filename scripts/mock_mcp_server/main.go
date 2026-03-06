package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	addr := os.Getenv("MCP_TEST_SERVER_ADDR")
	if addr == "" {
		addr = ":18081"
	}

	srv := mcpserver.NewMCPServer(
		"mock-mcp",
		"1.0.0",
		mcpserver.WithToolCapabilities(true),
	)

	srv.AddTool(
		mcp.NewTool(
			"echo",
			mcp.WithDescription("回显文本"),
			mcp.WithString("text", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("echo:" + req.GetString("text", "")), nil
		},
	)

	srv.AddTool(
		mcp.NewTool(
			"sum",
			mcp.WithDescription("计算两个数字之和"),
			mcp.WithNumber("a", mcp.Required()),
			mcp.WithNumber("b", mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			a := req.GetFloat("a", 0)
			b := req.GetFloat("b", 0)
			return mcp.NewToolResultText(fmt.Sprintf(`{"sum":%.0f}`, a+b)), nil
		},
	)

	httpServer := mcpserver.NewStreamableHTTPServer(
		srv,
		mcpserver.WithStateful(true),
	)

	log.Printf("mock mcp server listening on %s/mcp", addr)
	if err := httpServer.Start(addr); err != nil {
		log.Fatal(err)
	}
}
