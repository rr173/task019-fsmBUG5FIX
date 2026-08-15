// Package httpapi 提供工作流状态机引擎的 HTTP 接口。
// 服务无内部可变状态，可被多个 goroutine 复用。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"task019-fsm/internal/fsm"
)

// ErrBadJSON 表示请求体不是单个合法 JSON 对象。
var ErrBadJSON = errors.New("请求体不是合法的单个 JSON 对象")

// API 是工作流状态机服务的 HTTP 接口实现。
type API struct{}

// New 创建服务实例。
func New() *API { return &API{} }

// Handler 返回 HTTP 路由。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /validate", a.validate)
	mux.HandleFunc("POST /apply", a.apply)
	mux.HandleFunc("POST /path", a.path)
	mux.HandleFunc("POST /terminals", a.terminals)
	return mux
}

// decodeJSON 解码单个 JSON 对象，拒绝多段 JSON 与未知字段。
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return ErrBadJSON
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// outcome 用于校验端点与各类 400 错误的统一回应。
type outcome struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// applyRequest 内嵌工作流定义，并附带当前状态与事件标签。
type applyRequest struct {
	fsm.Definition
	State string `json:"state"`
	Event string `json:"event"`
}

// applyResponse 是推进端点的回应。无论是否推进都返回当前状态。
type applyResponse struct {
	State string `json:"state"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// pathRequest 内嵌工作流定义，并附带源与目标状态。
type pathRequest struct {
	fsm.Definition
	From string `json:"from"`
	To   string `json:"to"`
}

// pathResponse 是可达端点的回应。events 为空数组（非 null）表示空序列。
type pathResponse struct {
	Reachable bool     `json:"reachable"`
	Events    []string `json:"events"`
}

// terminalsResponse 是终态端点的回应。无终态时返回空数组。
type terminalsResponse struct {
	Terminals []string `json:"terminals"`
}

func (a *API) validate(w http.ResponseWriter, r *http.Request) {
	var def fsm.Definition
	if err := decodeJSON(r, &def); err != nil {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: err.Error()})
		return
	}
	if err := def.Validate(); err != nil {
		// 语义错误：请求格式正确，定义本身不合法，返回 200 + ok=false。
		writeJSON(w, http.StatusOK, outcome{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, outcome{OK: true})
}

func (a *API) apply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, applyResponse{OK: false, Error: err.Error()})
		return
	}
	if err := req.Definition.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, applyResponse{OK: false, Error: err.Error()})
		return
	}
	if !req.Definition.HasState(req.State) {
		writeJSON(w, http.StatusBadRequest, applyResponse{OK: false, Error: fmt.Sprintf("未知状态 %q", req.State)})
		return
	}
	next, ok, reason := req.Definition.Apply(req.State, req.Event)
	// 未定义转移或终态拒绝：请求格式正确，转移不被允许，返回 200 + ok=false。
	writeJSON(w, http.StatusOK, applyResponse{State: next, OK: ok, Error: reason})
}

func (a *API) path(w http.ResponseWriter, r *http.Request) {
	var req pathRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: err.Error()})
		return
	}
	if err := req.Definition.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: err.Error()})
		return
	}
	if !req.Definition.HasState(req.From) {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: fmt.Sprintf("未知源状态 %q", req.From)})
		return
	}
	if !req.Definition.HasState(req.To) {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: fmt.Sprintf("未知目标状态 %q", req.To)})
		return
	}
	reach, events := req.Definition.Path(req.From, req.To)
	if events == nil {
		events = []string{} // 空序列用 [] 而非 null
	}
	writeJSON(w, http.StatusOK, pathResponse{Reachable: reach, Events: events})
}

func (a *API) terminals(w http.ResponseWriter, r *http.Request) {
	var def fsm.Definition
	if err := decodeJSON(r, &def); err != nil {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: err.Error()})
		return
	}
	if err := def.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, outcome{OK: false, Error: err.Error()})
		return
	}
	terms := def.Terminals()
	if terms == nil {
		terms = []string{}
	}
	writeJSON(w, http.StatusOK, terminalsResponse{Terminals: terms})
}
