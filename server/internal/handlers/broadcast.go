package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ovh-buy/server/internal/app"
	"github.com/ovh-buy/server/internal/monitor"
)

// 定时播报的手动入口。
//
// 每日循环在 monitor 包里；这里额外的两个口子解决的是"配完不敢等第二天"
// 的问题：先 preview 看文案，再 run 真发一次。

type broadcastRequest struct {
	Queue   *bool `json:"queue"`
	Monitor *bool `json:"monitor"`
	VPS     *bool `json:"vps"`
	Report  *bool `json:"report"`
}

// selectionFromRequest 缺省用当前配置的分类开关，显式传值优先。
// 用 *bool 而不是 bool：前端"只预览队列"和"预览全部"必须能区分。
func selectionFromRequest(state *app.State, req broadcastRequest) monitor.TaskBroadcastSelection {
	cfg := state.Config.Get()
	selection := monitor.TaskBroadcastSelection{
		Queue:   cfg.IsTaskBroadcastQueueEnabled(),
		Monitor: cfg.IsTaskBroadcastMonitorEnabled(),
		VPS:     cfg.IsTaskBroadcastVPSEnabled(),
		Report:  cfg.IsTaskBroadcastReportEnabled(),
	}
	if req.Queue != nil {
		selection.Queue = *req.Queue
	}
	if req.Monitor != nil {
		selection.Monitor = *req.Monitor
	}
	if req.VPS != nil {
		selection.VPS = *req.VPS
	}
	if req.Report != nil {
		selection.Report = *req.Report
	}
	return selection
}

// RunTaskBroadcast 立即发送一次播报，返回实际发出的消息条数。
func RunTaskBroadcast(state *app.State, mon *monitor.Monitor) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req broadcastRequest
		// 允许空 body：不传就按当前配置全发。
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求体格式不正确"})
				return
			}
		}
		selection := selectionFromRequest(state, req)
		sent := mon.SendTaskBroadcast(c.Request.Context(), selection)
		c.JSON(http.StatusOK, gin.H{"status": "success", "sent": sent})
	}
}

// PreviewTaskBroadcast 只构建不发送，用于确认文案和价格。
// 预览也受分类开关影响，避免"预览里有监控、实际播报没有"这种错位。
func PreviewTaskBroadcast(state *app.State, mon *monitor.Monitor) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req broadcastRequest
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "请求体格式不正确"})
				return
			}
		}
		selection := selectionFromRequest(state, req)
		messages := mon.BuildTaskBroadcastMessages(c.Request.Context(), selection)
		if selection.Report {
			if report := mon.BuildBattleReport(); report != "" {
				messages = append(messages, report)
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "success", "messages": messages})
	}
}
