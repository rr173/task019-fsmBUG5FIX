# task019-fsm

工作流状态机引擎服务，仅使用标准库实现：接收工作流定义（状态集合、初始状态、带标签的有向转移集合），提供定义校验、事件推进、有向可达性与终态计算。所有判定只依据定义本身，不持久化、不记录实例历史，不依赖任何第三方库、数据库或外部服务。

## 本地运行

```bash
go run . server --addr :8080
go run . --smoke-test
```

主要接口（请求体均为单个 JSON 对象，未知字段与多段 JSON 会被拒绝）：

- `GET /healthz`：健康检查。
- `POST /validate`：提交工作流定义 `{"name":...,"states":[...],"initial":"...","transitions":[{"from":"...","event":"...","to":"..."}]}`，返回 `{"ok":bool,"error":"..."}`。语义错误（重复转移、未声明状态等）返回 200 + `ok=false`。
- `POST /apply`：在定义上追加 `"state"` 与 `"event"`，返回 `{"state":"...","ok":bool,"error":"..."}`。未定义转移或终态拒绝返回 200 + `ok=false` 且 `state` 不变；未知状态或非法定义返回 400。
- `POST /path`：在定义上追加 `"from"` 与 `"to"`，返回 `{"reachable":bool,"events":[...]}`（最短有向事件序列）。未知状态或非法定义返回 400。
- `POST /terminals`：提交定义，返回 `{"terminals":[...]}`（按名排序）。非法定义返回 400。

边界约束要点：

- 未定义转移与终态锁定：`(state,event)` 未定义时拒绝并点名状态与事件；终态拒绝所有事件，且终态错误与未定义错误文字可区分；两者均返回 200、不改变状态。
- 定义确定性：同一 `(from,event)` 不得重复（无论目标是否相同）；转移引用未声明状态、初始状态不在状态集合、状态集合为空或含重复名均判 `ok=false`。
- 有向可达性：仅沿转移方向 BFS 求最短事件序列；正向不可达返回 `reachable=false`；源等于目标返回可达与空序列。
- 终态由出度判定：出度为零即终态，与入度无关；纯环路无终态返回空数组（合法）。

## Docker

镜像使用国内 DaoCloud Go 1.26.3 Bookworm builder 和 Alpine 3.20 runtime；支持 `linux/amd64` 与 `linux/arm64` 双架构。
