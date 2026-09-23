package router

import (
	"hazop-safeguard-coverage/backend/internal/constants"
	"hazop-safeguard-coverage/backend/internal/handler"
	"hazop-safeguard-coverage/backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterSafeguardOutageRoutes(api *gin.RouterGroup, h *handler.SafeguardOutageHandler) {
	group := api.Group("/safeguard-outages")
	group.GET("", middleware.RequirePermission(constants.PermissionRead), h.List)
	group.GET("/:id", middleware.RequirePermission(constants.PermissionRead), h.Get)
	manage := middleware.RequirePermission(constants.PermissionOutage)
	group.POST("/safeguards/:safeguardId", manage, h.Register)
	group.POST("/:id/revoke", manage, h.Revoke)
}
