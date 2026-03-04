package handler

import (
	"errors"
	"net/http"

	"github.com/devonlyian/go-event-commerce/services/order-service/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type OrderHandler struct {
	orderService *service.OrderService
	logger       *zap.Logger
}

type createOrderRequest struct {
	CustomerID string  `json:"customer_id" binding:"required"`
	Amount     float64 `json:"amount" binding:"required,gt=0"`
}

func NewOrderHandler(orderService *service.OrderService, logger *zap.Logger) *OrderHandler {
	return &OrderHandler{orderService: orderService, logger: logger}
}

func (h *OrderHandler) Register(r *gin.Engine) {
	r.POST("/orders", h.createOrder)
	r.GET("/orders/:id", h.getOrderByID)
}

func (h *OrderHandler) createOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "detail": err.Error()})
		return
	}

	order, err := h.orderService.CreateOrder(c.Request.Context(), service.CreateOrderInput{
		CustomerID: req.CustomerID,
		Amount:     req.Amount,
	})
	if err != nil {
		var publishErr *service.EventPublishError
		if errors.As(err, &publishErr) {
			h.logger.Warn("order persisted but event publish failed", zap.Error(err), zap.String("order_id", order.ID))
			c.JSON(http.StatusAccepted, gin.H{
				"order":   order,
				"warning": "order persisted but event publish failed",
			})
			return
		}

		h.logger.Error("failed to create order", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create order"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"order": order})
}

func (h *OrderHandler) getOrderByID(c *gin.Context) {
	id := c.Param("id")
	order, err := h.orderService.GetOrderByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
			return
		}
		h.logger.Error("failed to get order", zap.Error(err), zap.String("order_id", id))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch order"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"order": order})
}
