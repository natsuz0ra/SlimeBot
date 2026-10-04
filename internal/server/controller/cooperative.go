package controller

import (
	"context"
	"github.com/google/uuid"
	"net/http"
	"path/filepath"
	"slimebot/internal/domain"
	sbruntime "slimebot/internal/runtime"
	workspace "slimebot/internal/runtime/workspace"
	subagent "slimebot/internal/services/subagent"
	"strconv"
	"time"
)

func (h *HTTPController) SetCooperativeService(s *subagent.Service) { h.cooperative = s }

func (h *HTTPController) ApproveCooperative(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	var p struct {
		Approved *bool `json:"approved"`
	}
	if c.ShouldBindJSON(&p) != nil || p.Approved == nil {
		jsonError(c, http.StatusBadRequest, "Approval decision required")
		return
	}
	status := "rejected"
	if *p.Approved {
		status = "approved"
	}
	approval, err := h.cooperative.Store.GetAgentApproval(c.Request().Context(), c.Param("id"))
	if err != nil {
		jsonError(c, http.StatusNotFound, "Approval not found")
		return
	}
	if err := h.cooperative.Store.ResolveAgentApproval(c.Request().Context(), c.Param("id"), status); err != nil {
		jsonError(c, http.StatusConflict, err.Error())
		return
	}
	h.cooperative.Runtime.Emit(c.Request().Context(), approval.RootID, approval.AgentID, approval.TurnID, "approval_resolved", map[string]string{"id": c.Param("id"), "status": status})
	c.JSON(http.StatusOK, map[string]string{"status": status})
}
func (h *HTTPController) CooperativeArtifact(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	var p struct {
		Command string `json:"command"`
		RootID  string `json:"rootId"`
		Action  string `json:"action"`
	}
	if c.ShouldBindJSON(&p) != nil || p.RootID == "" {
		jsonError(c, http.StatusBadRequest, "Root required")
		return
	}
	ctx := c.Request().Context()
	artifacts, err := h.cooperative.Store.ListAgentArtifacts(ctx, p.RootID)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	var artifact *domain.AgentArtifact
	for i := range artifacts {
		if artifacts[i].ID == c.Param("id") {
			artifact = &artifacts[i]
			break
		}
	}
	if artifact == nil {
		jsonError(c, http.StatusNotFound, "Artifact not found")
		return
	}
	manager := workspace.New(filepath.Join(sbruntime.SlimeBotHomeDir(), "agent-worktrees"))
	if p.Action == "inspect" {
		diff, err := manager.Diff(ctx, *artifact)
		if err != nil {
			jsonError(c, http.StatusConflict, err.Error())
			return
		}
		c.JSON(http.StatusOK, map[string]any{"artifact": artifact, "diff": diff})
		return
	}
	if p.Action == "validate" {
		if err = h.cooperative.ValidateArtifact(ctx, artifact, p.Command); err != nil {
			jsonError(c, http.StatusConflict, err.Error())
			return
		}
		if err = h.cooperative.Store.PutAgentArtifact(ctx, artifact); err != nil {
			jsonInternalError(c, err)
			return
		}
		h.cooperative.Runtime.Emit(ctx, p.RootID, artifact.AgentID, "", "artifact_validated", artifact)
		c.JSON(http.StatusOK, map[string]any{"artifact": artifact})
		return
	}
	if p.Action != "integrate" {
		jsonError(c, http.StatusBadRequest, "Invalid action")
		return
	}
	err = h.cooperative.Integrate(ctx, artifact)
	if err != nil {
		jsonError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, map[string]any{"artifact": artifact})
}
func (h *HTTPController) ListCooperative(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
	ctx := c.Request().Context()
	if waitMS, _ := strconv.Atoi(c.Query("wait")); waitMS > 0 {
		changed := h.cooperative.Runtime.Changes()
		events, e := h.cooperative.Store.ListAgentEvents(ctx, c.Param("id"), after)
		if e == nil && len(events) == 0 {
			_ = h.cooperative.Runtime.WaitSignal(ctx, changed, time.Duration(min(waitMS, 30000))*time.Millisecond)
		}
	}
	x, err := h.cooperative.Snapshot(ctx, c.Param("id"), after)
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, x)
}
func (h *HTTPController) AgentAction(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	var p struct {
		Action    string `json:"action"`
		Message   string `json:"message"`
		ClientID  string `json:"clientMessageId"`
		Delivery  string `json:"delivery"`
		RequestID string `json:"requestId"`
		Scope     string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&p); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid agent action")
		return
	}
	ctx := c.Request().Context()
	d, err := h.cooperative.Store.GetAgent(ctx, c.Param("id"))
	if err != nil {
		jsonError(c, http.StatusNotFound, "Agent not found")
		return
	}
	caller := subagent.Caller{AgentID: d.RootID, RootID: d.RootID, RequestID: p.RequestID}
	if p.Action == "interrupt" {
		if err = h.cooperative.Runtime.Interrupt(ctx, d.SessionID, p.Scope == "subtree"); err != nil {
			jsonInternalError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, map[string]string{"status": "stopping"})
		return
	}
	if p.Action != "message" {
		jsonError(c, http.StatusBadRequest, "Invalid action")
		return
	}
	if caller.RequestID == "" {
		caller.RequestID = d.RequestID
		if active, _, ok := h.cooperative.Runtime.CurrentRoot(d.RootID); ok {
			caller.RequestID = active
		}
	}
	rootCtx, live := h.cooperative.Runtime.RootContext(caller.RequestID)
	standalone := false
	if !live {
		caller.RequestID = uuid.NewString()
		rootCtx, err = h.cooperative.Runtime.BeginRoot(context.WithoutCancel(ctx), d.RootID, caller.RequestID)
		if err != nil {
			jsonError(c, http.StatusConflict, err.Error())
			return
		}
		standalone = true
	}
	m, err := h.cooperative.Send(ctx, caller, d.SessionID, p.Message, p.ClientID, p.Delivery, true)
	if err != nil {
		if standalone {
			_ = h.cooperative.Runtime.FinishRoot(context.WithoutCancel(ctx), caller.RequestID, true)
		}
		jsonError(c, http.StatusConflict, err.Error())
		return
	}
	if standalone {
		go func() {
			for rootCtx.Err() == nil && h.cooperative.Runtime.HasWork(rootCtx, d.RootID, caller.RequestID) {
				_ = h.cooperative.Runtime.Wait(rootCtx, d.RootID, nil, time.Second)
			}
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(rootCtx), 5*time.Second)
			defer cancel()
			_ = h.cooperative.Runtime.FinishRoot(finishCtx, caller.RequestID, rootCtx.Err() != nil)
		}()
	}
	c.JSON(http.StatusAccepted, m)
}
func (h *HTTPController) CooperativeTask(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	var p subagent.TaskMutation
	if err := c.ShouldBindJSON(&p); err != nil {
		jsonError(c, http.StatusBadRequest, "Invalid task mutation")
		return
	}
	root := c.Param("id")
	ctx := c.Request().Context()
	request, rootCtx, active := h.cooperative.Runtime.CurrentRoot(root)
	standalone := false
	if p.Action == "reassign" && !active && p.OwnerID != "" && p.OwnerID != root {
		request = uuid.NewString()
		var e error
		rootCtx, e = h.cooperative.Runtime.BeginRoot(context.WithoutCancel(ctx), root, request)
		if e != nil {
			jsonError(c, http.StatusConflict, e.Error())
			return
		}
		standalone = true
	}
	t, err := h.cooperative.Task(ctx, subagent.Caller{AgentID: root, RootID: root, RequestID: request}, p)
	if standalone {
		if err != nil {
			_ = h.cooperative.Runtime.FinishRoot(context.WithoutCancel(ctx), request, true)
		} else {
			go h.finishStandalone(rootCtx, root, request)
		}
	}
	if err != nil {
		jsonError(c, http.StatusConflict, err.Error())
		return
	}
	c.JSON(http.StatusOK, t)
}
func (h *HTTPController) GetAgentTurns(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	id := c.Param("id")
	if _, err := h.cooperative.Store.GetAgent(c.Request().Context(), id); err != nil {
		jsonError(c, http.StatusNotFound, "Agent not found")
		return
	}
	var t []domain.AgentTurn
	var err error
	if turnID := c.Query("turnId"); turnID != "" {
		var turn *domain.AgentTurn
		turn, err = h.cooperative.Store.GetAgentTurn(c.Request().Context(), id, turnID)
		if err == nil {
			t = append(t, *turn)
		}
	} else {
		t, err = h.cooperative.Store.ListAgentTurns(c.Request().Context(), id)
	}
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if t == nil {
		t = []domain.AgentTurn{}
	}
	c.JSON(http.StatusOK, t)
}

func (h *HTTPController) StopCooperative(c WebContext) {
	if h.cooperative == nil {
		jsonError(c, http.StatusServiceUnavailable, "Cooperative runtime unavailable")
		return
	}
	h.cooperative.Runtime.StopRoot(c.Param("id"))
	c.JSON(http.StatusAccepted, map[string]string{"status": "stopping"})
}

func (h *HTTPController) finishStandalone(ctx context.Context, root, request string) {
	for ctx.Err() == nil && h.cooperative.Runtime.HasWork(ctx, root, request) {
		_ = h.cooperative.Runtime.Wait(ctx, root, nil, time.Second)
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = h.cooperative.Runtime.FinishRoot(finish, request, ctx.Err() != nil)
}
