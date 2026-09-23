package router

import (
	"hazop-safeguard-coverage/backend/internal/constants"
	"hazop-safeguard-coverage/backend/internal/handler"
	"hazop-safeguard-coverage/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterSafeguardOutageRoutes(api *gin.RouterGroup, h *handler.SafeguardOutageHandler) {
	read := middleware.RequirePermission(constants.PermissionRead)
	write := middleware.RequirePermission(constants.PermissionSafeguardOutage)
	api.GET("/safeguard-outages", read, h.List)
	api.POST("/safeguard-outages/:id/revoke", write, h.Revoke)
	api.GET("/safeguards/:id/outages", read, h.ListForSafeguard)
	api.POST("/safeguards/:id/outages", write, h.Register)
}
