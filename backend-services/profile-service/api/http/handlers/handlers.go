package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"profile-service/internal/service"
	
	"profile-service/internal/domain"

	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"

	"github.com/jackc/pgx/v5"
)

type Handler struct {
	Srv *service.Service
}

func NewHandler(srv *service.Service) *Handler {
	return &Handler{Srv: srv}
}

func (h *Handler) RegisterRoutes(r *http.ServeMux) {
	r.HandleFunc("POST /api/profile/", h.CreateProfile)
	r.HandleFunc("GET /api/profile/{id}", h.GetProfile)
	r.HandleFunc("GET /api/profile/username/{username}", h.GetProfileByUsername)
	r.HandleFunc("PUT /api/profile/", h.UpdateProfile)
	r.HandleFunc("DELETE /api/profile/", h.DeleteProfile)
	r.HandleFunc("PUT /api/profile/avatar/", h.UpdateAvatar)
	r.HandleFunc("GET /api/profile/avatar/{id}", h.GetAvatar)
}

func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var profReq domain.CreateProfileRequest
	err := json.NewDecoder(r.Body).Decode(&profReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := r.Header.Get("X-User-ID")
	if id == "" {
		http.Error(w, "missing user id", http.StatusUnauthorized)
		return
	}
	prof, err := h.Srv.CreateProfile(r.Context(), &profReq, id)
	if err != nil {
		if err == domain.ErrUsernameTaken {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, "failed to create profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prof)
}

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == ""{
		http.Error(w, "user id is required", http.StatusBadRequest)
		return
	}
	prof, err := h.Srv.GetProfile(r.Context(), id)

	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "ID not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(prof)
}

func (h *Handler) GetProfileByUsername(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == ""{
		http.Error(w, "username is required", http.StatusBadRequest)
	}

	usrResponse, err := h.Srv.GetProfileByUsername(r.Context(), username)
	if err != nil {
		if err == domain.ErrProfileNotFound {
			http.Error(w, domain.ErrProfileNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(usrResponse)
}


func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userId := r.Header.Get("X-User-ID")

	var prof domain.Profile
	if decodeErr := json.NewDecoder(r.Body).Decode(&prof); decodeErr != nil {
		http.Error(w, decodeErr.Error(), http.StatusBadRequest)
		return
	}

	prof.ID = userId

	err := h.Srv.UpdateProfile(r.Context(), &prof)

	if err != nil {
		if err == domain.ErrProfileNotFound {
			http.Error(w, domain.ErrProfileNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update profile", http.StatusInternalServerError)
		return
	}
	metrics.ProfileUpdatesTotal.Inc()

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	err := h.Srv.DeleteProfile(r.Context(), userID)

	if err != nil {
		if err == domain.ErrProfileNotFound {
			http.Error(w, domain.ErrProfileNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete profile", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")

	r.Body = http.MaxBytesReader(w, r.Body, 5 << 20)

	data, err := io.ReadAll(r.Body)
	if err != nil{
		http.Error(w, "File too large or read error", http.StatusRequestEntityTooLarge)
		return
	}

	err = h.Srv.UpdateAvatar(r.Context(), userID, data)

	if err != nil {
		if err == domain.ErrProfileNotFound {
			http.Error(w, "Profile not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update avatar", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetAvatar(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	avatar, err := h.Srv.GetAvatar(r.Context(), userID)

	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "ID not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get avatar", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Cache-Control", "public, max-age=86400") 

	w.Header().Set("Content-Type", "image/jpeg")
	w.Write(avatar)
}
