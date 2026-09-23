package handler

import (
	"github.com/gin-gonic/gin"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/service"
	"hazop-safeguard-coverage/backend/internal/util"
	"net/http"
)

type SafeguardOutageHandler struct {
	service service.SafeguardOutageService
}

func NewSafeguardOutageHandler(value service.SafeguardOutageService) *SafeguardOutageHandler {
	return &SafeguardOutageHandler{service: value}
}

func (h *SafeguardOutageHandler) List(c *gin.Context) {
	safeguardID, ok := optionalUint(c, "safeguard_id")
	if !ok {
		return
	}
	scenarioID, ok := optionalUint(c, "scenario_id")
	if !ok {
		return
	}
	page, size := util.Pagination(c)
	status := c.Query("status")
	if status != "" && status != "active" && status != "pending" && status != "ended" {
		util.Fail(c, util.NewError(http.StatusBadRequest, util.CodeBadRequest, "status must be one of active, pending, ended"))
		return
	}
	result, err := h.service.List(c.Request.Context(), dto.SafeguardOutageQuery{
		SafeguardID: safeguardID, ScenarioID: scenarioID, Status: status,
		Page: page, PageSize: size,
	})
	respond(c, http.StatusOK, result, err)
}

func (h *SafeguardOutageHandler) Get(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	result, err := h.service.Get(c.Request.Context(), id)
	respond(c, http.StatusOK, result, err)
}

func (h *SafeguardOutageHandler) Register(c *gin.Context) {
	safeguardID, err := util.ParseUintParam(c, "safeguardId")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.RegisterSafeguardOutageRequest
	if !bindJSON(c, &request) {
		return
	}
	result, serviceErr := h.service.Register(c.Request.Context(), safeguardID, request, mustActor(c))
	respond(c, http.StatusCreated, result, serviceErr)
}

func (h *SafeguardOutageHandler) Revoke(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.RevokeSafeguardOutageRequest
	if !bindJSON(c, &request) {
		return
	}
	result, serviceErr := h.service.Revoke(c.Request.Context(), id, request, mustActor(c))
	respond(c, http.StatusOK, result, serviceErr)
}
