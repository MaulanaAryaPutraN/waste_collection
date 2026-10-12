package response

import (
	"encoding/json"
	"net/http"
)

type Error struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Envelope struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	Data    any      `json:"data,omitempty"`
	Meta    any      `json:"meta,omitempty"`
	Errors  *[]Error `json:"errors,omitempty"`
}

func JSON(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func Success(w http.ResponseWriter, status int, message string, data, meta any) {
	JSON(w, status, Envelope{Success: true, Message: message, Data: data, Meta: meta})
}

func Failure(w http.ResponseWriter, status int, message string, issues ...Error) {
	if issues == nil {
		issues = []Error{}
	}
	JSON(w, status, Envelope{Success: false, Message: message, Errors: &issues})
}
