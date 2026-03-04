package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	readinessCheck func(ctx context.Context) error
}

func NewHealthHandler(readinessCheck func(ctx context.Context) error) *HealthHandler {
	return &HealthHandler{readinessCheck: readinessCheck}
}

func (h *HealthHandler) Register(r *gin.Engine) {
	r.GET("/livez", h.liveness)
	r.GET("/readyz", h.readiness)
}

func (h *HealthHandler) liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *HealthHandler) readiness(c *gin.Context) {
	if h.readinessCheck == nil {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.readinessCheck(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
