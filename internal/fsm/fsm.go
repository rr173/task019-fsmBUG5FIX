// Package fsm 实现工作流状态机引擎：定义校验、事件推进、有向可达性与终态计算。
// 所有判定只依据定义本身，不持久化、不记录实例历史。
package fsm

import (
	"errors"
	"fmt"
	"strings"
)

// Transition 是一条带标签的有向转移：在 From 状态收到 Event 即转移到 To。
type Transition struct {
	From  string `json:"from"`
	Event string `json:"event"`
	To    string `json:"to"`
}

// Definition 是工作流状态机的定义。
type Definition struct {
	Name        string       `json:"name"`
	States      []string     `json:"states"`
	Initial     string       `json:"initial"`
	Transitions []Transition `json:"transitions"`
}

// Validate 校验定义的合法性与确定性，返回第一处违规。
// 检查项：名称非空、状态集合非空且不重复、初始状态属于状态集合、
// 每条转移的起始与目标均属于状态集合且字段完整、
// 同一 (起始, 事件) 不得重复（工作流必须确定性）。
func (d Definition) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("工作流名称不能为空")
	}
	if len(d.States) == 0 {
		return errors.New("状态集合不能为空")
	}
	seen := make(map[string]bool, len(d.States))
	for _, s := range d.States {
		if seen[s] {
			return fmt.Errorf("状态名重复：%q", s)
		}
		seen[s] = true
	}
	if !seen[d.Initial] {
		return fmt.Errorf("初始状态 %q 不在状态集合中", d.Initial)
	}
	type tkey struct{ from, event string }
	seenT := make(map[tkey]bool, len(d.Transitions))
	for i, t := range d.Transitions {
		if strings.TrimSpace(t.From) == "" || strings.TrimSpace(t.Event) == "" || strings.TrimSpace(t.To) == "" {
			return fmt.Errorf("第 %d 条转移字段缺失", i)
		}
		if !seen[t.From] {
			return fmt.Errorf("第 %d 条转移的起始 %q 不在状态集合", i, t.From)
		}
		if !seen[t.To] {
			return fmt.Errorf("第 %d 条转移的目标 %q 不在状态集合", i, t.To)
		}
		k := tkey{t.From, t.Event}
		if seenT[k] {
			return fmt.Errorf("重复转移：起始 %q 收到事件 %q 已定义", t.From, t.Event)
		}
		seenT[k] = true
	}
	return nil
}

// HasState 报告 s 是否为已声明状态。调用前应已 Validate。
func (d *Definition) HasState(s string) bool {
	for _, x := range d.States {
		if x == s {
			return true
		}
	}
	return false
}

// IsTerminal 报告 state 是否为终态（出度为零，不作为任何转移的起始），
// 与其入度无关。
func (d *Definition) IsTerminal(state string) bool {
	for _, t := range d.Transitions {
		if t.From == state {
			return false
		}
	}
	return true
}

// Terminals 返回所有终态（出度为零），按名称升序排序。
func (d *Definition) Terminals() []string {
	out := make([]string, 0, len(d.States))
	for _, s := range d.States {
		if d.IsTerminal(s) {
			out = append(out, s)
		}
	}
	return out
}

// Apply 判定从 state 发起 event 的结果。
// 返回 (下一状态, 是否推进, 拒绝原因)。ok=true 时 next 为目标状态；
// ok=false 时 next==state（状态不变）且 reason 描述拒绝原因：
//   - state 不在状态集合：返回"未知状态"原因；
//   - state 为终态：返回"终态"原因（无论 event 是什么）；
//   - (state, event) 未定义：返回"未定义转移"原因，并点名 state 与 event。
//
// 调用前应已 Validate。
func (d *Definition) Apply(state, event string) (next string, ok bool, reason string) {
	if !d.HasState(state) {
		return "", false, fmt.Sprintf("未知状态 %q", state)
	}
	if d.IsTerminal(state) {
		return state, false, fmt.Sprintf("终态 %q 不响应任何事件", state)
	}
	for _, t := range d.Transitions {
		if t.From == state && t.Event == event {
			return t.To, true, ""
		}
	}
	return state, false, fmt.Sprintf("未定义转移：状态 %q 收到事件 %q", state, event)
}

// Path 返回从 from 到 to 的最短有向事件标签序列（广度优先，沿转移方向）。
// from==to 时返回 (true, nil)（可达，空序列）；
// 目标沿正向不可达时返回 (false, nil)。
// 调用前应已 Validate 且 from、to ∈ states。
func (d *Definition) Path(from, to string) (bool, []string) {
	adj := make(map[string][]Transition, len(d.States))
	for _, t := range d.Transitions {
		adj[t.From] = append(adj[t.From], t)
	}
	visited := map[string]bool{from: true}
	type item struct {
		state  string
		events []string
	}
	queue := []item{{state: from}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, t := range adj[cur.state] {
			if t.To == to {
				return true, append(append([]string{}, cur.events...), t.Event)
			}
			if !visited[t.To] {
				visited[t.To] = true
				queue = append(queue, item{
					state:  t.To,
					events: append(append([]string{}, cur.events...), t.Event),
				})
			}
		}
	}
	return false, nil
}
