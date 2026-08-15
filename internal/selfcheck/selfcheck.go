// Package selfcheck 提供无需外部依赖的自检：通过 httptest 启动真实 HTTP
// 服务，覆盖校验、推进、可达、终态端点与各边界约束。成功返回 0，任一失败返回 1。
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"task019-fsm/internal/httpapi"
)

// Run 执行自检并返回退出码。
func Run() int {
	passed, failed := 0, 0
	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			failed++
			fmt.Printf("FAIL %-36s %v\n", name, err)
		} else {
			passed++
			fmt.Printf("PASS %s\n", name)
		}
	}

	srv := httptest.NewServer(httpapi.New().Handler())
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, []byte, error) {
		var r io.Reader
		if body != "" {
			r = bytes.NewReader([]byte(body))
		}
		req, err := http.NewRequest(method, srv.URL+path, r)
		if err != nil {
			return nil, nil, err
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, data, readErr
	}

	// orderDef 构造一份合法的订单履约工作流定义（map 形式，便于拼装请求体）。
	orderDef := func() map[string]any {
		return map[string]any{
			"name":    "order-fulfillment",
			"states":  []string{"pending", "paid", "shipped", "delivered", "cancelled"},
			"initial": "pending",
			"transitions": []map[string]string{
				{"from": "pending", "event": "pay", "to": "paid"},
				{"from": "paid", "event": "ship", "to": "shipped"},
				{"from": "shipped", "event": "deliver", "to": "delivered"},
				{"from": "pending", "event": "cancel", "to": "cancelled"},
				{"from": "paid", "event": "cancel", "to": "cancelled"},
			},
		}
	}
	marshal := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return string(b)
	}

	// 各端点请求/响应封装。
	validate := func(body string) (int, bool, string, error) {
		resp, data, err := do(http.MethodPost, "/validate", body)
		if err != nil {
			return 0, false, "", err
		}
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &out)
		return resp.StatusCode, out.OK, out.Error, nil
	}
	apply := func(body string) (int, string, bool, string, error) {
		resp, data, err := do(http.MethodPost, "/apply", body)
		if err != nil {
			return 0, "", false, "", err
		}
		var out struct {
			State string `json:"state"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &out)
		return resp.StatusCode, out.State, out.OK, out.Error, nil
	}
	path := func(body string) (int, bool, []string, error) {
		resp, data, err := do(http.MethodPost, "/path", body)
		if err != nil {
			return 0, false, nil, err
		}
		var out struct {
			Reachable bool     `json:"reachable"`
			Events    []string `json:"events"`
		}
		_ = json.Unmarshal(data, &out)
		return resp.StatusCode, out.Reachable, out.Events, nil
	}
	terminals := func(body string) (int, []string, error) {
		resp, data, err := do(http.MethodPost, "/terminals", body)
		if err != nil {
			return 0, nil, err
		}
		var out struct {
			Terminals []string `json:"terminals"`
		}
		_ = json.Unmarshal(data, &out)
		return resp.StatusCode, out.Terminals, nil
	}

	// ---- 健康检查 ----
	check("健康检查", func() error {
		resp, _, err := do(http.MethodGet, "/healthz", "")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	// ---- 校验端点 ----
	check("校验合法定义", func() error {
		status, ok, errStr, err := validate(marshal(orderDef()))
		if err != nil {
			return err
		}
		if status != http.StatusOK || !ok {
			return fmt.Errorf("status=%d ok=%v err=%s", status, ok, errStr)
		}
		return nil
	})

	check("校验重复转移(异目标)", func() error {
		m := orderDef()
		m["transitions"] = append(m["transitions"].([]map[string]string),
			map[string]string{"from": "pending", "event": "pay", "to": "shipped"})
		status, ok, errStr, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		if !strings.Contains(errStr, "pending") || !strings.Contains(errStr, "pay") {
			return fmt.Errorf("error should name state+event, got: %q", errStr)
		}
		return nil
	})

	check("校验重复转移(同目标)", func() error {
		m := orderDef()
		m["transitions"] = append(m["transitions"].([]map[string]string),
			map[string]string{"from": "pending", "event": "pay", "to": "paid"})
		status, ok, _, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		return nil
	})

	check("校验转移目标不在状态集合", func() error {
		m := orderDef()
		m["transitions"] = append(m["transitions"].([]map[string]string),
			map[string]string{"from": "paid", "event": "x", "to": "ghost"})
		status, ok, _, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		return nil
	})

	check("校验初始状态不在状态集合", func() error {
		m := orderDef()
		m["initial"] = "nowhere"
		status, ok, _, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		return nil
	})

	check("校验状态集合为空", func() error {
		m := orderDef()
		m["states"] = []string{}
		status, ok, _, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		return nil
	})

	check("非法 JSON 被拒(400)", func() error {
		status, _, _, err := validate("{not json")
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		return nil
	})

	check("多段 JSON 被拒(400)", func() error {
		status, _, _, err := validate(marshal(orderDef()) + marshal(orderDef()))
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		return nil
	})

	check("未知字段被拒(400)", func() error {
		m := orderDef()
		m["extra"] = 1
		status, _, _, err := validate(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		return nil
	})

	// ---- 推进端点 ----
	applyBody := func(state, event string) string {
		m := orderDef()
		m["state"] = state
		m["event"] = event
		return marshal(m)
	}

	check("推进正常转移", func() error {
		status, state, ok, errStr, err := apply(applyBody("pending", "pay"))
		if err != nil {
			return err
		}
		if status != http.StatusOK || !ok || state != "paid" {
			return fmt.Errorf("status=%d ok=%v state=%q err=%s want 200/true/paid", status, ok, state, errStr)
		}
		return nil
	})

	check("推进未定义事件不改变状态(200)", func() error {
		// paid 有 ship/cancel，deliver 未定义。
		status, state, ok, errStr, err := apply(applyBody("paid", "deliver"))
		if err != nil {
			return err
		}
		if status != http.StatusOK || ok {
			return fmt.Errorf("status=%d ok=%v want 200/false", status, ok)
		}
		if state != "paid" {
			return fmt.Errorf("state should be unchanged, got %q", state)
		}
		if !strings.Contains(errStr, "paid") || !strings.Contains(errStr, "deliver") {
			return fmt.Errorf("error should name state+event, got: %q", errStr)
		}
		if !strings.Contains(errStr, "未定义转移") {
			return fmt.Errorf("error should say undefined transition, got: %q", errStr)
		}
		return nil
	})

	check("终态拒绝所有事件(200)", func() error {
		// delivered 是终态。
		for _, ev := range []string{"pay", "ship", "deliver", "cancel", "anything"} {
			status, state, ok, errStr, err := apply(applyBody("delivered", ev))
			if err != nil {
				return err
			}
			if status != http.StatusOK || ok {
				return fmt.Errorf("event %q: status=%d ok=%v want 200/false", ev, status, ok)
			}
			if state != "delivered" {
				return fmt.Errorf("event %q: state changed to %q", ev, state)
			}
			if !strings.Contains(errStr, "终态") || !strings.Contains(errStr, "delivered") {
				return fmt.Errorf("event %q: error should mention terminal+state, got: %q", ev, errStr)
			}
		}
		return nil
	})

	check("终态错误与未定义错误相区分", func() error {
		_, _, _, terminalErr, err1 := apply(applyBody("delivered", "pay"))
		if err1 != nil {
			return err1
		}
		_, _, _, undefinedErr, err2 := apply(applyBody("paid", "deliver"))
		if err2 != nil {
			return err2
		}
		if terminalErr == undefinedErr {
			return fmt.Errorf("terminal and undefined errors must differ: both=%q", terminalErr)
		}
		if !strings.Contains(terminalErr, "终态") {
			return fmt.Errorf("terminal error missing 终态: %q", terminalErr)
		}
		if !strings.Contains(undefinedErr, "未定义转移") {
			return fmt.Errorf("undefined error missing 未定义转移: %q", undefinedErr)
		}
		return nil
	})

	check("推进未知状态(400)", func() error {
		status, _, ok, _, err := apply(applyBody("ghost", "pay"))
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		if ok {
			return fmt.Errorf("ok=true want false")
		}
		return nil
	})

	check("推进非法定义(400)", func() error {
		m := orderDef()
		m["initial"] = "nowhere"
		m["state"] = "pending"
		m["event"] = "pay"
		status, _, _, _, err := apply(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		return nil
	})

	// ---- 可达端点 ----
	pathBody := func(from, to string) string {
		m := orderDef()
		m["from"] = from
		m["to"] = to
		return marshal(m)
	}

	check("可达直接转移", func() error {
		status, reach, events, err := path(pathBody("pending", "cancelled"))
		if err != nil {
			return err
		}
		if status != http.StatusOK || !reach {
			return fmt.Errorf("status=%d reach=%v want 200/true", status, reach)
		}
		if len(events) != 1 || events[0] != "cancel" {
			return fmt.Errorf("events=%v want [cancel]", events)
		}
		return nil
	})

	check("可达多跳最短", func() error {
		status, reach, events, err := path(pathBody("pending", "delivered"))
		if err != nil {
			return err
		}
		if status != http.StatusOK || !reach {
			return fmt.Errorf("status=%d reach=%v want 200/true", status, reach)
		}
		want := []string{"pay", "ship", "deliver"}
		if len(events) != len(want) {
			return fmt.Errorf("events=%v want %v", events, want)
		}
		for i := range want {
			if events[i] != want[i] {
				return fmt.Errorf("events=%v want %v", events, want)
			}
		}
		return nil
	})

	check("可达自身返回空序列", func() error {
		status, reach, events, err := path(pathBody("pending", "pending"))
		if err != nil {
			return err
		}
		if status != http.StatusOK || !reach {
			return fmt.Errorf("status=%d reach=%v want 200/true", status, reach)
		}
		if len(events) != 0 {
			return fmt.Errorf("events=%v want empty", events)
		}
		return nil
	})

	check("正向不可达(反向可达也不行)", func() error {
		// delivered 是终态，delivered->pending 正向不可达。
		status, reach, _, err := path(pathBody("delivered", "pending"))
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("status=%d want 200", status)
		}
		if reach {
			return fmt.Errorf("delivered->pending should be unreachable forward")
		}
		return nil
	})

	check("可达未知源(400)", func() error {
		status, _, _, err := path(pathBody("ghost", "pending"))
		if err != nil {
			return err
		}
		if status != http.StatusBadRequest {
			return fmt.Errorf("status=%d want 400", status)
		}
		return nil
	})

	// ---- 终态端点 ----
	check("终态列表(按名排序)", func() error {
		status, terms, err := terminals(marshal(orderDef()))
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("status=%d want 200", status)
		}
		want := []string{"cancelled", "delivered"}
		if len(terms) != len(want) {
			return fmt.Errorf("terminals=%v want %v", terms, want)
		}
		for i := range want {
			if terms[i] != want[i] {
				return fmt.Errorf("terminals=%v want %v", terms, want)
			}
		}
		return nil
	})

	check("终态无(纯环路)返回空数组", func() error {
		m := map[string]any{
			"name":    "cycle",
			"states":  []string{"a", "b"},
			"initial": "a",
			"transitions": []map[string]string{
				{"from": "a", "event": "x", "to": "b"},
				{"from": "b", "event": "y", "to": "a"},
			},
		}
		status, terms, err := terminals(marshal(m))
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("status=%d want 200", status)
		}
		if len(terms) != 0 {
			return fmt.Errorf("cycle should have no terminals, got %v", terms)
		}
		return nil
	})

	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}
