package service

import (
	"context"
	"net/http"

	"github.com/mikasa/mcp-manager/internal/domain/entity"
	"github.com/mikasa/mcp-manager/internal/mcpclient"
	"github.com/mikasa/mcp-manager/internal/repository"
	"github.com/mikasa/mcp-manager/pkg/response"
)

func normalizeToolActionError(ctx context.Context, repo repository.MCPServiceRepository, service *entity.MCPService, actionMessage string, err error) error {
	if !mcpclient.IsSessionReconnectRequired(err) {
		return response.NewBizError(http.StatusBadGateway, response.CodeToolInvokeFailed, actionMessage, err)
	}

	if repo != nil && service != nil {
		next := service.FailureCount + 1
		if next <= 0 {
			next = 1
		}
		_ = repo.UpdateStatus(ctx, service.ID, entity.ServiceStatusError, next, err.Error())
	}

	return response.NewBizError(http.StatusConflict, response.CodeConflict, "会话已失效，请重新连接服务", err)
}
