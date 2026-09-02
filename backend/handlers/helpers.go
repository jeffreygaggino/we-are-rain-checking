// Package handlers is the HTTP edge. It never touches the database: a handler parses the request,
// calls a service, maps the error, and writes the envelope.
//
// There is no permission step at the top of a handler body (ADR-0001), so the seven-step body from
// STRUCTURE starts at step 2 here.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
)

// errorResponse and successResponse are the only two ways a handler writes a body. Everything the
// API returns is one of these two shapes, including the 404 for an unmatched route.
func errorResponse(c *gin.Context, status int, msg string) {
	c.JSON(status, models.ErrorResponse{Success: false, Code: status, Message: msg})
}

func successResponse(c *gin.Context, status int, msg string, data interface{}) {
	c.JSON(status, models.HttpResponse{Success: true, Code: status, Message: msg, Data: data})
}

// NotFound answers any path the router does not know. Gin's own 404 is plain text, which would make
// "every response is an envelope" true of the routes and false of the API.
func NotFound(c *gin.Context) {
	errorResponse(c, http.StatusNotFound, "no such route: "+c.Request.Method+" "+c.Request.URL.Path)
}
