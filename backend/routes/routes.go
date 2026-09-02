// Package routes holds the one place a URL is bound to a handler.
//
// There is no CORS middleware and no auth middleware. Neither is an omission: there is no browser
// client to allow yet (#2), and there is no auth layer at all (ADR-0001).
package routes

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/jeffreygaggino/we-are-rain-checking/backend/docs"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/handlers"
)

// SetupRouter registers every route this service serves, the generated documentation included.
// See plans/01-backend-v1.md, Deviation 7.
func SetupRouter(healthHandler *handlers.HealthHandler) *gin.Engine {
	router := gin.Default()

	// Unmatched paths answer through the shared error envelope rather than gin's plain-text 404.
	router.NoRoute(handlers.NotFound)

	// The group is the literal path this service serves. API_BASE_PATH describes where a proxy
	// publishes it, and belongs to the generated spec rather than to routing.
	v1 := router.Group("/api/v1")
	{
		// Health
		v1.GET("/health", healthHandler.GetHealth)
	}

	// Generated API documentation. Never hand-edited — `make docs` regenerates it.
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return router
}
