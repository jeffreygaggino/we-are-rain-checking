package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/jeffreygaggino/we-are-rain-checking/backend/models"
	"github.com/jeffreygaggino/we-are-rain-checking/backend/services"
)

type SeasonHandler struct {
	seasonService *services.SeasonService
}

func NewSeasonHandler(seasonService *services.SeasonService) *SeasonHandler {
	return &SeasonHandler{seasonService: seasonService}
}

// GetSeason returns one season's Driver-Races with each Race's weather, plus its cancelled Races.
//
//	@Summary		A season's Races
//	@Description	Every Race of the season in date order, each carrying its Weather Samples as raw counts and aggregates, and every Driver's end result. No threshold is applied to the weather: there is no Wet Session flag and no wind band, and the caller decides what wet and windy mean. A Race that was cancelled is included with cancelled=true, null weather and no results — there was nothing to observe, and the calendar slot is still worth reporting. Returned whole, not paged.
//	@Tags			seasons
//	@Produce		json
//	@Param			year	path		int	true	"Season, within the range this service carries"
//	@Success		200		{object}	models.HttpResponse{data=models.Season}
//	@Failure		400		{object}	models.ErrorResponse
//	@Failure		500		{object}	models.ErrorResponse
//	@Router			/seasons/{year}/races [get]
func (h *SeasonHandler) GetSeason(c *gin.Context) {
	year, err := strconv.Atoi(c.Param("year"))
	if err != nil {
		// Never echoes strconv's message, which quotes the raw input back at the caller.
		errorResponse(c, http.StatusBadRequest, "year must be a four-digit year")
		return
	}

	season, err := h.seasonService.Season(c.Request.Context(), year)
	if err != nil {
		// The one place this resource turns a sentinel into a status. Naming the range matters more
		// than the refusal: it tells the caller which years to ask for instead.
		if errors.Is(err, models.ErrSeasonOutOfRange) {
			first, last := services.SeasonRange()
			errorResponse(c, http.StatusBadRequest,
				fmt.Sprintf("season %d is outside the seasons this service carries (%d-%d)", year, first, last))
			return
		}

		log.Printf("season: %v", err)
		errorResponse(c, http.StatusInternalServerError, "could not read the season")
		return
	}

	successResponse(c, http.StatusOK, "season races", season)
}
