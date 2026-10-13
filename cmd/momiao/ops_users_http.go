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

func localOpsUsersRoute(path string) bool {
	return path == opsUsersPath || strings.HasPrefix(path, opsUsersPath+"/") || path == "/api/v1/ops/logs"
}
func newLocalOpsUsersHandler(store *platform.Store, auth *identity.HTTP) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := identity.UserFromContext(r.Context())
		if !ok {
			walletError(w, 401, "AUTH_UNAUTHORIZED")
			return
		}
		if user.Role < 10 {
			walletError(w, 403, "OPS_FORBIDDEN")
			return
		}
		if store == nil || auth == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		values := r.URL.Query()
		page, size := int64(1), int64(20)
		numbers := map[string]*int64{"page": &page, "page_size": &size}
		f := platform.OpsLogFilter{Model: values.Get("model"), Group: values.Get("group"), Dimension: values.Get("dimension")}
		for key, dst := range map[string]*int64{"start": &f.Start, "end": &f.End, "user": &f.User, "key": &f.Key, "channel": &f.Channel} {
			numbers[key] = dst
		}
		for key, dst := range numbers {
			if raw := values.Get(key); raw != "" {
				n, err := strconv.ParseInt(raw, 10, 64)
				if err != nil || n < 0 {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				*dst = n
			}
		}
		if page < 1 || page > 1000000 || size < 1 || size > 100 {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		var result any
		var err error
		switch {
		case r.URL.Path == "/api/v1/ops/logs" && r.Method == "GET":
			if f.Dimension == "" {
				f.Dimension = "model"
			}
			if f.Dimension != "model" && f.Dimension != "channel" && f.Dimension != "user" || f.End > 0 && f.Start > f.End {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			if raw := values.Get("errors_only"); raw != "" {
				f.ErrorsOnly, err = strconv.ParseBool(raw)
				if err != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
			}
			f.Page, f.PageSize = page, size
			result, err = store.LocalOpsLogs(ctx, f)
		case r.URL.Path == opsUsersPath && r.Method == "GET":
			filter := platform.OpsUserFilter{Page: page, PageSize: size, Search: values.Get("q"), Group: values.Get("group")}
			for key, dst := range map[string]*int{"role": &filter.Role, "status": &filter.Status} {
				if raw := values.Get(key); raw != "" {
					*dst, err = strconv.Atoi(raw)
					if err != nil {
						walletError(w, 400, "OPS_INPUT_INVALID")
						return
					}
				}
			}
			if filter.Role != 0 && filter.Role != 1 && filter.Role != 10 && filter.Role != 100 || filter.Status < 0 || filter.Status > 2 || len(filter.Search) > 100 {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = store.ListLocalOpsUsers(ctx, filter)
		case strings.HasPrefix(r.URL.Path, opsUsersPath+"/"):
			parts := strings.Split(strings.TrimPrefix(r.URL.Path, opsUsersPath+"/"), "/")
			id, e := strconv.ParseInt(parts[0], 10, 64)
			if e != nil || id <= 0 || len(parts) > 2 {
				walletError(w, 404, "NOT_FOUND")
				return
			}
			action := ""
			if len(parts) == 2 {
				action = parts[1]
			}
			switch {
			case action == "" && r.Method == "GET":
				result, err = store.LocalOpsUser(ctx, id)
			case action == "ledger" && r.Method == "GET":
				result, err = store.LocalOpsLedger(ctx, id, page, size)
			case action == "keys" && r.Method == "GET":
				result, err = store.ListAPIKeys(ctx, id, int(page), int(size))
			case action == "master" && r.Method == "GET":
				result, err = store.LocalOpsMaster(ctx, id)
			case action == "" && r.Method == "PUT" || r.Method == "POST" && (action == "logout" || action == "reset-2fa" || action == "reserve" || action == "disable-key" || action == "delete-key" || action == "master-rename-required" || action == "master-force-rename"):
				var v platform.OpsUserChange
				if json.NewDecoder(r.Body).Decode(&v) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				if action == "" {
					action = "profile"
				}
				err = store.ChangeLocalOpsUser(ctx, auth.Sessions, user.ID, id, action, v)
			default:
				walletError(w, 404, "NOT_FOUND")
				return
			}
		default:
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if err != nil {
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				walletError(w, 404, "NOT_FOUND")
			case errors.Is(err, platform.ErrWalletNotFound):
				writeJSONError(w, 409, "用户尚未初始化钱包")
			case errors.Is(err, platform.ErrOpsLastSuperAdmin):
				writeJSONError(w, 409, "至少保留一位启用的最高管理员")
			case errors.Is(err, platform.ErrInsufficientBalance):
				writeJSONError(w, 409, "Reserve 余额不足")
			case errors.Is(err, platform.ErrBalanceOverflow):
				writeJSONError(w, 400, "金额超出上限")
			case errors.Is(err, platform.ErrIdempotencyConflict):
				writeJSONError(w, 409, "调账编号已用于另一笔操作")
			case errors.Is(err, platform.ErrNicknameTaken), errors.Is(err, platform.ErrNicknameReserved), errors.Is(err, platform.ErrInvalidNickname):
				writeProfileError(w, err)
			default:
				writeOpsError(w, err)
			}
			return
		}
		walletSuccess(w, result)
	})
}
