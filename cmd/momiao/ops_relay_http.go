package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cy4268/momiao/internal/identity"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/relay"
	"github.com/jackc/pgx/v5/pgconn"
)

func opsRelayRoute(path string) bool {
	return path == "/api/v1/ops/channels" || strings.HasPrefix(path, "/api/v1/ops/channels/") || path == "/api/v1/ops/groups" || strings.HasPrefix(path, "/api/v1/ops/groups/")
}
func newOpsRelayHandler(store *platform.Store) http.Handler {
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
		if store == nil {
			walletError(w, 503, "OPS_UNAVAILABLE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		f := platform.OpsRelayFilter{Page: 1, PageSize: 20, Search: r.URL.Query().Get("q"), Group: r.URL.Query().Get("group"), Enabled: r.URL.Query().Get("enabled")}
		for name, dst := range map[string]*int64{"page": &f.Page, "page_size": &f.PageSize, "type": &f.Type} {
			if raw := r.URL.Query().Get(name); raw != "" {
				n, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				*dst = n
			}
		}
		if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 || !slices.Contains([]int64{0, 1, 14, 24, 41, 43}, f.Type) || f.Enabled != "" && f.Enabled != "true" && f.Enabled != "false" {
			walletError(w, 400, "OPS_INPUT_INVALID")
			return
		}
		var result any
		var err error
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/ops/")
		switch {
		case path == "channels" && r.Method == "GET":
			result, err = store.ListOpsChannels(ctx, f)
		case path == "channels" && r.Method == "PUT":
			var v platform.ChannelWrite
			if json.NewDecoder(r.Body).Decode(&v) != nil || platform.ValidateChannel(&v) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = store.SaveOpsChannel(ctx, user.ID, v)
		case path == "channels/models" && r.Method == "POST":
			var v platform.ChannelWrite
			if json.NewDecoder(r.Body).Decode(&v) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			v.Name = "model discovery"
			if platform.ValidateChannel(&v) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			var channel platform.RelayChannel
			if v.ID > 0 && v.Key == "" {
				channel, _, err = store.OpsProbeChannel(ctx, v.ID, "")
			}
			if err == nil {
				channel.Type, channel.BaseURL = v.Type, v.BaseURL
				channel.HeaderOverride, _ = json.Marshal(v.HeaderOverride)
				if v.Key != "" {
					channel.Key = v.Key
				}
				result, err = relay.FetchChannelModels(ctx, channel)
				if err != nil {
					writeJSONError(w, 502, err.Error())
					return
				}
			}
		case strings.HasPrefix(path, "channels/"):
			parts := strings.Split(strings.TrimPrefix(path, "channels/"), "/")
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
				result, err = store.OpsChannel(ctx, id)
			case action == "" && r.Method == "DELETE":
				err = store.ChangeOpsChannel(ctx, user.ID, id, nil)
			case action == "" && r.Method == "PATCH":
				var v platform.ChannelPatch
				if json.NewDecoder(r.Body).Decode(&v) != nil || platform.ValidateChannelPatch(v) != nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				result, err = store.PatchOpsChannel(ctx, user.ID, id, v)
			case action == "status" && r.Method == "PUT":
				var v struct {
					Enabled *bool `json:"enabled"`
				}
				if json.NewDecoder(r.Body).Decode(&v) != nil || v.Enabled == nil {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				err = store.ChangeOpsChannel(ctx, user.ID, id, v.Enabled)
			case (action == "test" || action == "models") && r.Method == "POST":
				var channel platform.RelayChannel
				var options relay.ChannelTestOptions
				if action == "test" && (json.NewDecoder(r.Body).Decode(&options) != nil || !options.Valid()) {
					walletError(w, 400, "OPS_INPUT_INVALID")
					return
				}
				if action == "test" {
					// Keep the normal ops budget for every other action.
					var cancelTest context.CancelFunc
					ctx, cancelTest = context.WithTimeout(r.Context(), relay.RequestTimeout)
					defer cancelTest()
					_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(relay.RequestTimeout + 30*time.Second))
				}
				channel, options.Model, err = store.OpsProbeChannel(ctx, id, strings.TrimSpace(options.Model))
				if err == nil {
					if action == "test" {
						probe := relay.TestChannel(ctx, channel, options)
						result = probe
						// Persist a timeout result after its business context expires.
						recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
						err = store.RecordChannelTest(recordCtx, id, probe.TestAt, probe.Duration)
						recordCancel()
					} else {
						result, err = relay.FetchChannelModels(ctx, channel)
						if err != nil {
							writeJSONError(w, 502, err.Error())
							return
						}
					}
				}
			default:
				walletError(w, 405, "METHOD_NOT_ALLOWED")
				return
			}
		case path == "groups" && r.Method == "GET":
			result, err = store.OpsGroups(ctx)
		case path == "groups" && r.Method == "PUT":
			var v platform.GroupWrite
			if json.NewDecoder(r.Body).Decode(&v) != nil || platform.ValidateGroup(&v) != nil {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			result, err = store.SaveOpsGroup(ctx, user.ID, v)
		case path == "groups" && r.Method == "DELETE":
			var v struct {
				Name string `json:"name"`
			}
			if json.NewDecoder(r.Body).Decode(&v) != nil || v.Name == "" {
				walletError(w, 400, "OPS_INPUT_INVALID")
				return
			}
			err = store.DeleteOpsGroup(ctx, user.ID, v.Name)
		case path == "groups/availability" && r.Method == "GET":
			result, err = store.RelayAvailability(ctx, f)
		default:
			walletError(w, 404, "NOT_FOUND")
			return
		}
		if err != nil {
			var pgerr *pgconn.PgError
			if errors.Is(err, platform.ErrRelayGroupInUse) {
				writeJSONError(w, 409, err.Error())
				return
			}
			if errors.As(err, &pgerr) && pgerr.Code == "23505" {
				writeJSONError(w, 409, "分组名称已存在")
				return
			}
			writeOpsError(w, err)
			return
		}
		walletSuccess(w, result)
	})
}
