package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
)

type HealthHandler struct {
	healthService *services.HealthService
}

func NewHealthHandler(healthService *services.HealthService) *HealthHandler {
	return &HealthHandler{healthService: healthService}
}

// GetHealth reports whether this service's dependencies answer.
//
//	@Summary		Health check
//	@Description	Reports database connectivity, not process liveness: a 200 means the database answered.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	models.HttpResponse{data=models.HealthReport}
//	@Failure		503	{object}	models.ErrorResponse
//	@Router			/health [get]
func (h *HealthHandler) GetHealth(c *gin.Context) {
	report, err := h.healthService.Check(c.Request.Context())
	if err != nil {
		// The driver's own message goes to the log, never to the client. The client gets the name of
		// the dependency that failed, which is the part that tells them where to look.
		log.Printf("health: %v", err)
		errorResponse(c, http.StatusServiceUnavailable, "database unreachable")
		return
	}

	successResponse(c, http.StatusOK, "service healthy", report)
}
