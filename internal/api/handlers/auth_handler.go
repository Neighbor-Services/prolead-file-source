package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gostore/internal/core/domain"
	"gostore/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totpCode,omitempty"`
}

type Verify2FARequest struct {
	TempToken string `json:"tempToken"`
	TOTPCode  string `json:"totpCode"`
}

type Enable2FARequest struct {
	Code   string `json:"code"`
	Secret string `json:"secret"`
}

type Disable2FARequest struct {
	Code string `json:"code"`
}

type CreateStaffRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UpdateStaffRequest struct {
	Role   string `json:"role"`
	Status string `json:"status"`
}

type ResetPasswordRequest struct {
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	result, err := h.authService.Login(r.Context(), req.Username, req.Password, req.TOTPCode)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *AuthHandler) Verify2FA(w http.ResponseWriter, r *http.Request) {
	var req Verify2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	result, err := h.authService.Verify2FALogin(r.Context(), req.TempToken, req.TOTPCode)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *AuthHandler) Setup2FA(w http.ResponseWriter, r *http.Request) {
	user := h.getAuthenticatedUser(r)
	if user == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	res, err := h.authService.Setup2FA(r.Context(), user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (h *AuthHandler) Enable2FA(w http.ResponseWriter, r *http.Request) {
	user := h.getAuthenticatedUser(r)
	if user == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req Enable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.authService.VerifyAndEnable2FA(r.Context(), user.ID, req.Code, req.Secret); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Two-Factor Authentication enabled successfully",
	})
}

func (h *AuthHandler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	user := h.getAuthenticatedUser(r)
	if user == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req Disable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if err := h.authService.Disable2FA(r.Context(), user.ID, req.Code); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Two-Factor Authentication disabled successfully",
	})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-API-Key")
	}
	token = extractBearer(token)

	if token != "" {
		h.authService.Logout(token)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user := h.getAuthenticatedUser(r)
	if user == nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// Staff Handlers
func (h *AuthHandler) ListStaff(w http.ResponseWriter, r *http.Request) {
	users, err := h.authService.ListStaff(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}

func (h *AuthHandler) CreateStaff(w http.ResponseWriter, r *http.Request) {
	var req CreateStaffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	user, err := h.authService.CreateStaff(r.Context(), req.Username, req.Email, req.Password, req.Role)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) UpdateStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req UpdateStaffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	user, err := h.authService.UpdateStaff(r.Context(), id, req.Role, req.Status)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) DeleteStaff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	currentUser := h.getAuthenticatedUser(r)
	if currentUser != nil && currentUser.ID == id {
		writeJSONError(w, http.StatusBadRequest, "Cannot delete your own active administrator account")
		return
	}

	if err := h.authService.DeleteStaff(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Staff account removed successfully",
	})
}

func (h *AuthHandler) ResetStaffPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if err := h.authService.ResetStaffPassword(r.Context(), id, req.Password); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password reset successfully",
	})
}

func (h *AuthHandler) getAuthenticatedUser(r *http.Request) *domain.User {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-API-Key")
	}
	token = extractBearer(token)
	if token == "" {
		return nil
	}
	return h.authService.ValidateToken(token)
}

func extractBearer(header string) string {
	if len(header) > 7 && (header[:7] == "Bearer " || header[:7] == "bearer ") {
		return header[7:]
	}
	return header
}
