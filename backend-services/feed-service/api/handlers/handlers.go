package handlers

import (
	"encoding/json"
	"feed-service/internal/domain"
	"feed-service/internal/service"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/abeb021/wave-connect/backend-services/pkg/metrics"
)

type Handler struct {
	Srv *service.Service
}

func NewHandler(srv *service.Service) *Handler {
	return &Handler{Srv: srv}
}

func (h *Handler) CreatePublication(w http.ResponseWriter, r *http.Request) {
	var pubReq domain.PublicationRequest
	err := json.NewDecoder(r.Body).Decode(&pubReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pubReq.UserID = r.Header.Get("X-User-ID")

	pub, err := h.Srv.CreatePublication(r.Context(), &pubReq)

	if err != nil {
		http.Error(w, "failed to create publication", http.StatusInternalServerError)
		return
	}

	metrics.FeedPublicationsTotal.Inc()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pub)
}

func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	pubs, err := h.Srv.GetFeedWithProfiles(r.Context())
	if err != nil {
		http.Error(w, "failed to get feed", http.StatusInternalServerError)
		return
	}

	if pubs == nil {
		pubs = []domain.FeedPublication{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pubs)
}

func (h *Handler) GetPublicationsByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")

	if userID == "" {
		http.Error(w, "user id is required", http.StatusBadRequest)
		return
	}

	pubs, err := h.Srv.GetPublicationsByUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to get feed", http.StatusInternalServerError)
		return
	}

	if pubs == nil {
		pubs = []domain.Publication{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pubs)
}

func (h *Handler) GetPublication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	pub, err := h.Srv.GetPublication(r.Context(), id)

	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "ID not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to get publication", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pub)
}

func (h *Handler) UpdatePublication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var pub domain.PublicationRequest
	if decodeErr := json.NewDecoder(r.Body).Decode(&pub); decodeErr != nil {
		http.Error(w, decodeErr.Error(), http.StatusBadRequest)
		return
	}

	pub.UserID = r.Header.Get("X-User-ID")

	err := h.Srv.UpdatePublication(r.Context(), id, pub.Text, pub.UserID)

	if err != nil {
		if err == domain.ErrPublicationNotFound {
			http.Error(w, "publication not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to update publication", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)

}

func (h *Handler) DeletePublication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := r.Header.Get("X-User-ID")

	err := h.Srv.DeletePublication(r.Context(), id, userID)

	if err != nil {
		if err == domain.ErrPublicationNotFound {
			http.Error(w, "publication not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete publication", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}


//COMMENTS
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	var commentReq domain.CommentRequest
	err := json.NewDecoder(r.Body).Decode(&commentReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	commentReq.UserID = r.Header.Get("X-User-ID")
	commentReq.PubID = r.PathValue("pubID")

	comment, err := h.Srv.CreateComment(r.Context(), &commentReq)

	if err != nil {
		if err == domain.ErrPublicationNotFound {
			http.Error(w, "Publication not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to create comment", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(comment)
}

func (h *Handler) GetCommentsByPublication(w http.ResponseWriter, r *http.Request) {
	pubID := r.PathValue("pubID")

	if pubID == "" {
		http.Error(w, "publication id is required", http.StatusBadRequest)
		return
	}

	comments, err := h.Srv.GetCommentsByPublication(r.Context(), pubID)
	if err != nil {
		http.Error(w, "failed to get comments", http.StatusInternalServerError)
		return
	}

	if comments == nil {
		comments = []domain.Comment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

func (h *Handler) DeleteComment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := r.Header.Get("X-User-ID")

	err := h.Srv.DeleteComment(r.Context(), id, userID)

	if err != nil {
		if err == domain.ErrCommentNotFound {
			http.Error(w, "comment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete comment", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}