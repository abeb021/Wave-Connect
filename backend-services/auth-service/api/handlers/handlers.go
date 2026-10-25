package handlers

import (
	"auth-service/internal/domain"
	"auth-service/internal/service"
	"encoding/json"
	"net/http"
	
	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
)

type Handler struct {
	Srv *service.Service
}

func NewHandler(srv *service.Service) *Handler {
	return &Handler{Srv: srv}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var usr domain.UserRequest
	err := json.NewDecoder(r.Body).Decode(&usr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	usrResponse, err := h.Srv.Register(r.Context(), &usr)
	if err != nil {
		if err == domain.ErrUserTaken {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usrResponse)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var usrReq domain.UserRequest
	err := json.NewDecoder(r.Body).Decode(&usrReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token, err := h.Srv.Login(r.Context(), &usrReq)
	if err != nil {
		if err == domain.ErrWrongPassword {
			metrics.AuthLoginsTotal.WithLabelValues("failed").Inc()
			http.Error(w, domain.ErrWrongPassword.Error(), http.StatusBadRequest)
			return
		}
		if err == domain.ErrUserNotFound {
			metrics.AuthLoginsTotal.WithLabelValues("failed").Inc()
			http.Error(w, domain.ErrUserNotFound.Error(), http.StatusNotFound)
			return
		}
		// database failed not the insuccesfull login
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	metrics.AuthLoginsTotal.WithLabelValues("success").Inc()

	w.Header().Set("Authorization", "Bearer "+token)
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) GetUserById(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-User-ID")
	if id == ""{
		http.Error(w, "missing user id", http.StatusUnauthorized)
		return
	}

	usrResponse, err := h.Srv.GetUserById(r.Context(), id)
	if err != nil {
		if err == domain.ErrUserNotFound {
			http.Error(w, domain.ErrUserNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(usrResponse)
}

func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.Header.Get("X-User-ID")
	if id == ""{
		http.Error(w, "missing user id", http.StatusUnauthorized)
		return
	}
	
	err := h.Srv.DeleteUser(r.Context(), id)
	if err != nil {
		if err == domain.ErrUserNotFound {
			http.Error(w, domain.ErrUserNotFound.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
