package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/service"
	"hazop-safeguard-coverage/backend/internal/util"
)

type SafeguardOutageHandler struct{ service service.SafeguardOutageService }

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
	result, err := h.service.List(c.Request.Context(), dto.SafeguardOutageQuery{
		SafeguardID: safeguardID, ScenarioID: scenarioID,
		Status: c.Query("status"), Page: page, PageSize: size,
	})
	respond(c, http.StatusOK, result, err)
}

func (h *SafeguardOutageHandler) ListForSafeguard(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	page, size := util.Pagination(c)
	result, serviceErr := h.service.List(c.Request.Context(), dto.SafeguardOutageQuery{
		SafeguardID: id, Status: c.Query("status"), Page: page, PageSize: size,
	})
	respond(c, http.StatusOK, result, serviceErr)
}

func (h *SafeguardOutageHandler) Register(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.RegisterSafeguardOutageRequest
	if !bindJSON(c, &request) {
		return
	}
	result, err := h.service.Register(c.Request.Context(), id, request, mustActor(c))
	respond(c, http.StatusCreated, result, err)
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
	result, err := h.service.Revoke(c.Request.Context(), id, request, mustActor(c))
	respond(c, http.StatusOK, result, err)
}
