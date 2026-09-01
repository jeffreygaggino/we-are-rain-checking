package models

import "math"

type ErrorResponse struct {
	Success bool   `json:"success" example:"false"`
	Code    int    `json:"code"    example:"400"`
	Message string `json:"message" example:"Failed response"`
}

type HttpResponse struct {
	Success bool        `json:"success"        example:"true"`
	Code    int         `json:"code,omitempty" example:"200"`
	Message string      `json:"message"        example:"Successful response"`
	Data    interface{} `json:"data,omitempty"`
}

// PaginationResponse is the paged envelope returned as HttpResponse.Data.
type PaginationResponse[T any] struct {
	FirstPage    bool `json:"firstPage"`
	LastPage     bool `json:"lastPage"`
	PageLength   int  `json:"pageLength"`
	TotalPages   int  `json:"totalPages"`
	TotalResults int  `json:"totalResults"`
	Data         []T  `json:"data"`
}

// NewPaginationResponse derives the flags from the page + total. `data` is never nil, so it
// encodes as [] rather than null.
func NewPaginationResponse[T any](data []T, page, pageLength, totalResults int) PaginationResponse[T] {
	if data == nil {
		data = []T{}
	}
	totalPages := 0
	if pageLength > 0 {
		totalPages = int(math.Ceil(float64(totalResults) / float64(pageLength)))
	}
	return PaginationResponse[T]{
		FirstPage:    page <= 1,
		LastPage:     page >= totalPages,
		PageLength:   pageLength,
		TotalPages:   totalPages,
		TotalResults: totalResults,
		Data:         data,
	}
}
