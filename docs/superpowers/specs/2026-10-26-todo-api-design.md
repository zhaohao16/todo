# Todo API v1 设计文档

- 日期：2026-10-26
- 状态：**待评审**（第一步产物，未写任何产品代码）
- 工具链：go1.27.3 windows/amd64
- 关联计划：`docs/superpowers/plans/2026-10-26-todo-api.md`

## 1. 目标

用 Go 标准库实现一个内存版 Todo REST API：五个端点（创建、列表、单条查询、修改、删除），JSON 出入，
参数校验给出合理状态码，业务逻辑与 HTTP 层都有单元测试，`go test ./...` 无需外部服务即可通过。

判据（Definition of Done）：

1. `go build ./...`、`go vet ./...`、`go test ./...`、`go test -race ./...` 全部通过。
2. `go.mod` 中没有任何 `require`（零第三方依赖）。
3. 每个端点都有 handler 层的表驱动测试覆盖成功路径与全部错误分支。
4. 校验规则在业务层（`internal/todo`）强制执行，而不是只写在 handler 里。

## 2. 范围

### 2.1 Goals

- 五个 REST 端点 + 统一错误信封。
- 内存存储，进程重启即丢失（明确接受）。
- 并发安全（多 goroutine 读写不 panic、不丢更新，`-race` 干净）。
- 分层清晰：domain/业务、存储、HTTP 适配器分离，业务层可脱离 HTTP 单测。

### 2.2 Non-goals（v1 明确不做）

- 数据库 / 持久化、迁移脚本。
- 认证、鉴权、多用户（无 `owner` 字段）。
- 分页、过滤、排序参数、全文搜索。
- 批量操作、软删除、回收站、`PUT` 全量替换。
- 限流、幂等键（`Idempotency-Key`）。
- OpenAPI 文档生成、Docker、CI 流水线。
- 优雅的 HTTP/2、TLS、CORS（仅记录 CORS 需要时如何加）。

### 2.3 Anti-goals（能跑但算失败）

- 共享 `map` 没有锁保护，靠“测试里刚好没并发”蒙过去。
- 错误响应格式一会儿 `{"error":"..."}` 一会儿纯文本。
- 校验只存在于 handler，任何人直接调 `Service` 就能写入非法数据。
- 测试只覆盖 happy path。
- 为了让测试好写而引入第三方断言/路由/模拟库。

## 3. 假设与决策（需你确认的已单列）

| 编号 | 决策 | 理由 |
|---|---|---|
| D1 | `id` 用 `int64` 自增，由内存存储分配 | 内存存储天然有序；测试可断言确定值；玩具项目不值得引入 UUID 依赖或手写 v4 |
| D2 | “修改”实现为 `PATCH` 部分更新，不提供 `PUT` | 客户端最常做的是“勾选完成”，`PUT` 全量替换会强迫它回传 `title`，容易丢字段；指针字段可以区分“未提供”和“零值” |
| D3 | `title` 必填，去空白后 1–200 个 **rune**；`description` 可选，≤2000 rune | 中文标题按字节计会误判；200 字符对一条待办足够 |
| D4 | 创建时允许显式传 `completed`（默认 `false`） | 便于导入/种子数据；与 PATCH 的模型保持一致 |
| D5 | 列表返回**裸 JSON 数组**，按 `id` 升序，空列表返回 `[]` 而非 `null` | 集合资源最直白的表示；`[]` 避免客户端判空分支。v1 不分页，分页是 v2 的破坏性变更点，已记录 |
| D6 | 请求体中出现未知字段 → `400` | `DisallowUnknownFields` 让客户端拼错字段名时立刻报错，而不是静默丢数据 |
| D7 | 校验失败统一 `400`，不使用 `422` | 单一语义，客户端只需处理一种；`422` 的分歧收益不足以抵消复杂度 |
| D8 | 错误响应统一为 `{"error":{"code","message","details"}}` | 客户端按 `code` 分支，`message` 只给人读；`details` 精确到字段 |
| D9 | 模块路径用占位符 `example.com/todo-api` | 当前没有远端仓库；将来定了仓库地址一次性替换 import 前缀 |
| D10 | 配置只有监听地址：`-addr` flag，缺省回落到 `TODO_ADDR`，再缺省 `:8080` | 只有一个配置项，单独建 `internal/config` 包是过度设计；已记录这是对通用模板的刻意偏离 |
| D11 | 额外提供 `GET /healthz` 与优雅关闭（`ReadHeaderTimeout` 等超时） | 不在需求内，但成本约 30 行，是“一个能跑的服务”和“一个能看的东西”的分界；可删除 |
| D12 | 时间字段 RFC3339（`time.Time` 默认编码），由存储注入 `clock` | 测试可注入固定时钟，不用 sleep 也不用 `time.Now()` 漂移 |
| D13 | 不提供 `Makefile` | 目标环境是 Windows，不保证有 `make`；`go` 命令直接写进 README |

### 3.1 待你确认的分歧点

- **A. 修改语义**：`PATCH`（推荐，见 D2）还是 `PUT` 全量替换？
- **B. `id` 类型**：`int64`（推荐，见 D1）还是 UUID 字符串？

其余决策（D3–D13）如果没有异议就按表中执行；任一条要改，改动会波及 API 契约与测试用例，越早越便宜。

## 4. API 设计

### 4.1 通用约定

- Base URL：`http://localhost:8080`
- 所有业务端点前缀 `/api/v1`（版本进路径，未来不兼容变更走 `/api/v2`）
- 资源名用复数名词、kebab-case：`/todos`
- JSON 字段统一 **camelCase**：`createdAt`、`updatedAt`
- 响应 `Content-Type: application/json; charset=utf-8`
- 成功响应直接返回资源或数组，不套 `data` 信封
- 请求体上限 **1 MiB**，超限 `413`
- `POST` / `PATCH` 要求 `Content-Type: application/json`（允许带 `charset`），否则 `415`
- 请求体必须恰好一个 JSON 对象；`null`、`[]`、多余尾随 token 一律 `400`

### 4.2 资源模型 `Todo`

```json
{
  "id": 1,
  "title": "买牛奶",
  "description": "低脂，2L",
  "completed": false,
  "createdAt": "2026-10-26T19:20:00+08:00",
  "updatedAt": "2026-10-26T19:20:00+08:00"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | int64 | 服务端分配，不可修改 |
| `title` | string | 必填，存储前去掉首尾空白 |
| `description` | string | 可选，缺省 `""`，保留原始换行 |
| `completed` | bool | 缺省 `false` |
| `createdAt` / `updatedAt` | RFC3339 string | 服务端分配；创建时两者相等 |

### 4.3 端点总览

| 方法 | 路径 | 成功状态 | 说明 |
|---|---|---|---|
| `POST` | `/api/v1/todos` | `201` | 创建，返回实体，带 `Location` 头 |
| `GET` | `/api/v1/todos` | `200` | 列表，按 `id` 升序 |
| `GET` | `/api/v1/todos/{id}` | `200` | 单条查询 |
| `PATCH` | `/api/v1/todos/{id}` | `200` | 部分更新，返回更新后实体 |
| `DELETE` | `/api/v1/todos/{id}` | `204` | 删除，无响应体 |
| `GET` | `/healthz` | `200` | 存活探针（D11） |

### 4.4 端点详情

#### POST /api/v1/todos

请求：

```json
{"title": "买牛奶", "description": "低脂，2L", "completed": false}
```

响应 `201 Created`，`Location: /api/v1/todos/1`，体为 4.2 的实体。

| 状态 | code | 场景 |
|---|---|---|
| 400 | `VALIDATION_ERROR` | `title` 缺失/空白/超长，`description` 超长，未知字段，JSON 语法错误 |
| 413 | `PAYLOAD_TOO_LARGE` | 请求体 > 1 MiB |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | 缺 `Content-Type` 或不是 `application/json` |

#### GET /api/v1/todos

响应 `200`：

```json
[{"id": 1, "title": "买牛奶", "description": "低脂，2L", "completed": false, "createdAt": "...", "updatedAt": "..."}]
```

空库返回 `[]`（不是 `null`）。无查询参数，无分页。

#### GET /api/v1/todos/{id}

| 状态 | code | 场景 |
|---|---|---|
| 200 | — | 命中 |
| 400 | `VALIDATION_ERROR` | `id` 不是正整数（如 `/todos/abc`、`/todos/0`、`/todos/-1`、`/todos/1.5`） |
| 404 | `NOT_FOUND` | 格式合法但不存在 |

#### PATCH /api/v1/todos/{id}

请求体所有字段可选，至少出现一个：

```json
{"completed": true}
```

语义：只修改出现的字段；`"title": null` 等同于“未提供”（Go 指针无法区分 `null` 与缺省，已记录）。

| 状态 | code | 场景 |
|---|---|---|
| 200 | — | 更新成功，返回完整实体，`updatedAt` 已刷新 |
| 400 | `VALIDATION_ERROR` | 空对象 `{}`、`title` 传了但空白/超长、`description` 超长、类型不符、未知字段 |
| 400 | `VALIDATION_ERROR` | `id` 非法 |
| 404 | `NOT_FOUND` | 目标不存在 |
| 413 / 415 | 同上 | 同上 |

#### DELETE /api/v1/todos/{id}

| 状态 | code | 场景 |
|---|---|---|
| 204 | — | 删除成功，无体 |
| 400 | `VALIDATION_ERROR` | `id` 非法 |
| 404 | `NOT_FOUND` | 目标不存在（不做幂等 204） |

#### GET /healthz

`200` + `{"status":"ok"}`。不检查任何依赖（内存存储无依赖可言），保持存活探针语义。

### 4.5 错误响应格式

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "invalid request body",
    "details": [
      {"field": "title", "message": "must be between 1 and 200 characters"}
    ]
  }
}
```

| code | HTTP | 含义 |
|---|---|---|
| `VALIDATION_ERROR` | 400 | 请求参数不合法，`details` 指出字段 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `UNSUPPORTED_MEDIA_TYPE` | 415 | `Content-Type` 不是 JSON |
| `PAYLOAD_TOO_LARGE` | 413 | 请求体超限 |
| `METHOD_NOT_ALLOWED` | 405 | 方法不支持（由 `http.ServeMux` 生成，见 8.1） |
| `INTERNAL` | 500 | 未预期错误，对外只回固定文案，细节只进日志 |

### 4.6 校验规则

| 字段 | 规则 | 违反时 |
|---|---|---|
| `title` | `TrimSpace` 后 `1 <= RuneCount <= 200` | 400，`details[0].field = "title"` |
| `description` | `RuneCount <= 2000`（创建与更新都校验） | 400 |
| `completed` | 必须是 JSON bool | 400（解码失败） |
| PATCH 体 | 至少一个字段非 `null`/非缺省 | 400，`message = "at least one field must be provided"` |
| 请求体 | ≤ 1 MiB，恰好一个 JSON 对象，无未知字段 | 413 / 400 |
| `id` 路径参数 | 纯十进制数字且 `> 0`，可解析为 int64 | 400 |

校验消息为英文短句，`code` 固定英文大写下划线；本文档中文说明，API 文本英文，避免编码/客户端匹配问题。

### 4.7 状态码决策说明

- 创建返回 `201` + `Location`：REST 惯例，客户端无需再查一次拿 id。
- 删除返回 `204`：无内容可回。
- `400` 而非 `422`：见 D7。
- 不存在的资源一律 `404`，不做“返回空对象 200”的宽容处理。
- `405` 由 `ServeMux` 的模式匹配自动产生并带 `Allow` 头（见 8.1 的已知不一致）。

## 5. 架构与目录结构

### 5.1 分层与依赖方向

```
cmd/todo-api  ──►  internal/httpapi  ──►  internal/todo  ◄──  internal/store
   (wiring)         (HTTP 适配器)          (实体+校验+用例)      (内存实现)
```

- `internal/todo` 定义自己需要的 `Store` 接口（**消费者侧接口**），因此它既不 import `store` 也不 import `httpapi`。
- `internal/store` 只 import `internal/todo` 拿实体类型，并断言 `var _ todo.Store = (*Memory)(nil)`。
- `internal/httpapi` 只 import `internal/todo`，负责 JSON、状态码、错误信封。
- `cmd/todo-api` 是唯一把三者接起来的地方；`main` 只做参数解析、装配、调用 `run`。

### 5.2 目录结构

```
todo.ds/
├── go.mod                      # module example.com/todo-api, go 1.27, 无 require
├── .gitignore                  # 二进制、覆盖率文件
├── README.md                   # 运行方式、curl 示例、设计要点
├── cmd/
│   └── todo-api/
│       └── main.go             # 薄 main：解析 -addr、装配、run(ctx) + 优雅关闭
├── internal/
│   ├── todo/                   # 业务核心，零基础设施依赖
│   │   ├── todo.go             # Todo / CreateInput / Patch / 校验 / ApplyPatch
│   │   ├── errors.go           # ErrNotFound、FieldError、ValidationError
│   │   ├── store.go            # Store 接口（消费者侧）
│   │   ├── service.go          # 用例编排：校验 → 调 store
│   │   ├── todo_test.go
│   │   └── service_test.go
│   ├── store/
│   │   ├── memory.go           # map + RWMutex + 自增 ID + 注入时钟
│   │   └── memory_test.go
│   └── httpapi/
│       ├── router.go           # ServeMux 注册 + 中间件串联
│       ├── handler.go          # 5 个 handler + /healthz
│       ├── decode.go           # 体积限制、Content-Type、严格解码、路径参数
│       ├── respond.go          # JSON 写入 + 错误信封 + 领域错误→状态码
│       ├── middleware.go       # recoverer（最外层）+ 请求日志
│       ├── handler_test.go
│       ├── decode_test.go
│       ├── respond_test.go
│       └── router_test.go
└── docs/superpowers/{specs,plans}/  # 本文档与实现计划
```

刻意**不创建**：`pkg/`、`api/`、`build/`、`internal/models`、`internal/utils`。没有外部消费者就没有 `pkg/`；
实体和校验放同一个 `todo` 包里，避免 `models` 变成垃圾桶。

### 5.3 Store 接口

```go
type Store interface {
	Create(ctx context.Context, t Todo) (Todo, error)          // 忽略 t.ID，分配 id/createdAt/updatedAt
	Get(ctx context.Context, id int64) (Todo, error)           // 不存在返回 ErrNotFound
	List(ctx context.Context) ([]Todo, error)                  // 按 id 升序
	Update(ctx context.Context, id int64, mutate func(*Todo)) (Todo, error) // 锁内原地修改并刷新 updatedAt
	Delete(ctx context.Context, id int64) error
}
```

`Update` 收一个突变函数而不是 `(id, Todo)`，是为了让“读—改—写”在同一把锁内完成：两个并发 PATCH
不会互相覆盖（否则 `Get` 之后 `Put` 之间会丢更新）。校验在进锁之前完成，锁内不做可能失败的逻辑。

## 6. 并发与一致性

- `Memory` 用 `sync.RWMutex`：`List`/`Get` 取读锁，`Create`/`Update`/`Delete` 取写锁。
- `id` 分配在写锁内自增，避免 `len(m)+1` 的重复。
- `Get`/`List` 返回**值拷贝**（`Todo` 是值类型，`List` 构造新切片），调用方拿不到内部状态，杜绝“从外面改掉存储里的对象”。
- `Update` 的 `mutate` 在写锁内执行，所以 PATCH 的读改写原子。
- 时钟注入：`NewMemory(now func() time.Time)`，测试传固定时钟，`main` 传 `time.Now`。

## 7. 测试策略

| 层 | 位置 | 内容 |
|---|---|---|
| 领域校验 | `internal/todo/todo_test.go` | 表驱动：空/纯空白/200 rune 边界/201 rune/中文与 emoji 计数/description 2000–2001 边界/空 PATCH |
| 用例 | `internal/todo/service_test.go` | 用 fake `Store` 覆盖：创建前校验、NotFound 透传、PATCH 只改提供的字段、空 PATCH 不触达 store |
| 存储 | `internal/store/memory_test.go` | CRUD、不存在、id 自增、列表顺序、注入时钟、并发读写（`-race`） |
| HTTP 适配 | `internal/httpapi/*_test.go` | `httptest` + 表驱动：每个端点成功与全部错误码、`Location` 头、错误信封结构、未知字段、多余尾随 token、超大体积、错误 Content-Type、405 与 `Allow`、panic → 500（recoverer） |
| 端到端 | `internal/httpapi/e2e_test.go` | 真实 service+store+router，跑一遍 创建→列表→查询→修改→删除→404 |

命令：

```
go build ./...
go vet ./...
gofmt -l .            # 无输出
go test ./...
go test -race ./...
go test -cover ./...
```

覆盖率目标：`internal/todo` 与 `internal/httpapi` ≥ 85%，`internal/store` ≥ 90%。不设 CI 硬门禁（v1 无 CI）。

## 8. 已知取舍与后续

### 8.1 不一致点（有意保留）

- `ServeMux` 对“路径存在但方法不对”返回 `405` 且响应体是纯文本、只带 `Allow` 头；未知路径是纯文本 `404`。
  与 4.5 的 JSON 信封不一致。要统一得自己包一层 `ResponseWriter` 或放弃 `ServeMux` 的方法路由，
  收益不抵成本，v1 保留 stdlib 行为并在 README 注明。

### 8.2 有意省略

- 分页：数据量小；将来加 `?limit=&cursor=` 时列表仍是数组，不破坏现有客户端。
- 乐观并发（`ETag`/`If-Match`）：`PATCH` 是原子读改写，v1 不做丢失更新的检测。
- 请求体 `null` 与 `{}` 都按校验失败处理，不特殊提示。

### 8.3 演进路径

- 换持久化：只替换 `todo.Store` 实现，`service`、`handler` 不动。
- 加 `PUT`：新增一个 handler，复用 `ValidateCreate` 的规则 + 保留 `id`/`createdAt`。
- 加认证：在中间件链里 `Auth` 位于 `Logger` 之后、handler 之前，`Todo` 加 `OwnerID`。

## 9. 待确认

1. 修改语义：`PATCH`（推荐）还是 `PUT`？
2. `id`：`int64`（推荐）还是 UUID？
3. D11（`/healthz` + 优雅关闭）保留还是删除？
