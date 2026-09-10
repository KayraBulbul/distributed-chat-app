package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/KayraBulbul/distributed-chat-app/backend/config"
	"github.com/KayraBulbul/distributed-chat-app/backend/server/response"
	"github.com/google/uuid"
)

func CreateUser(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var returnValue struct {
			Username string `json:"username"`
		}

		if err := json.NewDecoder(r.Body).Decode(&returnValue); err != nil {
			response.WithError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}

		user, err := cfg.Queries.CreateUser(r.Context(), returnValue.Username)
		if err != nil {
			response.WithError(w, http.StatusInternalServerError, "Error creating user")
			return
		}

		type responseValue struct {
			UserID    uuid.UUID `json:"user_id"`
			Username  string    `json:"username"`
			CreatedAt time.Time `json:"created_at"`
		}

		response.WithJSON(w, http.StatusOK, responseValue{
			UserID:    user.UserID,
			Username:  user.Username,
			CreatedAt: user.CreatedAt,
		})
	}
}
