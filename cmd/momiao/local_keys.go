// Official new-api@f1164142 token ownership and CRUD, adapted to local sessions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/jackc/pgx/v5"
)

func newLocalKeysHandler(store *platform.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		var data any
		var err error
		rest := strings.TrimPrefix(r.URL.Path, "/api/token/")
		if rest == "" {
			switch r.Method {
			case http.MethodGet:
				page, size := 1, 10
				for name, dest := range map[string]*int{"p": &page, "page_size": &size} {
					if raw := r.URL.Query().Get(name); raw != "" {
						*dest, err = strconv.Atoi(raw)
						if err != nil {
							walletError(w, 400, "KEY_INPUT_INVALID")
							return
						}
					}
				}
				if page < 1 || size < 1 || size > 100 {
					walletError(w, 400, "KEY_INPUT_INVALID")
					return
				}
				data, err = store.ListAPIKeys(ctx, user.ID, page, size)
			case http.MethodPost, http.MethodPut:
				var input platform.APIKeyInput
				if json.NewDecoder(r.Body).Decode(&input) != nil {
					walletError(w, 400, "KEY_INPUT_INVALID")
					return
				}
				mode := "edit"
				if r.Method == http.MethodPost {
					mode = "create"
				} else if r.URL.Query().Get("status_only") == "true" {
					mode = "status"
				} else if r.URL.Query().Get("purpose_only") == "true" {
					mode = "purpose"
				}
				input.Name = strings.TrimSpace(input.Name)
				valid := input.ID > 0 || mode == "create"
				if mode == "create" || mode == "edit" {
					valid = valid && input.Name != "" && len(input.Name) <= 50 && input.RemainQuota >= 0 && input.RemainQuota <= 1000000000000 && (input.ExpiredTime == -1 || input.ExpiredTime >= 0) && input.AllowIPs == "" && input.Group == "" && !input.CrossGroupRetry
				}
				if mode == "create" || mode == "purpose" {
					valid = valid && (input.Purpose == "GENERAL" || input.Purpose == "ROLEPLAY")
				}
				if mode == "status" {
					valid = valid && (input.Status == 1 || input.Status == 2)
				}
				if !valid {
					walletError(w, 400, "KEY_INPUT_INVALID")
					return
				}
				var item platform.APIKey
				item, err = store.SaveAPIKey(ctx, user.ID, input, mode)
				data = item
				if mode == "create" {
					data = map[string]string{"id": strconv.FormatInt(item.ID, 10)}
				}
			default:
				w.Header().Set("Allow", "GET, POST, PUT")
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		} else {
			reveal := strings.HasSuffix(rest, "/key")
			id, e := strconv.ParseInt(strings.TrimSuffix(rest, "/key"), 10, 64)
			if e != nil || id <= 0 {
				walletError(w, 404, "KEY_NOT_FOUND")
				return
			}
			switch {
			case reveal && r.Method == http.MethodPost:
				var key string
				key, err = store.RevealAPIKey(ctx, user.ID, id)
				data = map[string]string{"key": key}
			case !reveal && r.Method == http.MethodGet:
				data, err = store.ReadAPIKey(ctx, user.ID, id)
			case !reveal && r.Method == http.MethodDelete:
				err = store.DeleteAPIKey(ctx, user.ID, id)
			default:
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		}
		if err != nil {
			status, code := 503, "KEY_UNAVAILABLE"
			if errors.Is(err, pgx.ErrNoRows) {
				status, code = 404, "KEY_NOT_FOUND"
			} else if errors.Is(err, platform.ErrInvalidMutation) {
				status, code = 400, "KEY_INPUT_INVALID"
			} else if errors.Is(err, platform.ErrMaintenanceActive) {
				code = "MAINTENANCE_ACTIVE"
			}
			walletError(w, status, code)
			return
		}
		walletSuccess(w, data)
	})
}
