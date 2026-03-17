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

// liveness godoc
// @Summary Liveness probe
// @Description Returns OK when the process is alive.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponseDoc
// @Router /livez [get]
func (h *HealthHandler) liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readiness godoc
// @Summary Readiness probe
// @Description Returns ready when dependencies are reachable.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponseDoc
// @Failure 503 {object} ReadinessFailureResponseDoc
// @Router /readyz [get]
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
