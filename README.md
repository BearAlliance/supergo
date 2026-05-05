# supergo

A [supertest](https://github.com/forwardemail/supertest)-inspired HTTP testing library for Go.
Fluent, chainable assertions for your HTTP handlers and first-class support for stubbing the external services they call.

```go
supergo.New(handler).
    Post("/books").
    SendJSON(Book{Title: "Dune", Author: "Herbert"}).
    Expect(201).
    ExpectBodyContainsJSON("title", "Dune").
    Test(t)
```

## Install

```
go get github.com/bearalliance/supergo
```

Requires Go 1.22+.

## Testing your handler

See [example](./example) for a full example.

### One-shot requests

`New` wraps any `http.Handler` for a single request.
No state is shared between calls.

```go
supergo.New(handler).
    Get("/users").
    Expect(200).
    ExpectHeader("Content-Type", "application/json").
    ExpectBodyContainsJSON("0.name", "alice").
    Test(t)
```

### Stateful agent

`NewAgent` persists cookies across requests, making it natural for flows that require login.

```go
agent := supergo.NewAgent(handler)

agent.Post("/login").
    SendJSON(map[string]string{"username": "admin", "password": "secret"}).
    Expect(200).
    Test(t)

// Session cookie is sent automatically on subsequent requests.
agent.Post("/books").
    SendJSON(Book{Title: "Dune"}).
    Expect(201).
    Test(t)
```

### `http.Server` convenience

```go
supergo.NewServer(&http.Server{Handler: mux}).
	Get("/health").
	Expect(200).
	Test(t)
```

## Building requests

| Method                                                     | Description                                                       |
|------------------------------------------------------------|-------------------------------------------------------------------|
| `Get / Post / Put / Patch / Delete / Head / Options(path)` | Start a request                                                   |
| `.Set(key, value)`                                         | Add a header                                                      |
| `.Auth(username, password)`                                | HTTP Basic Auth                                                   |
| `.Send(body)`                                              | Send body (auto-detects JSON object/array vs plain text)          |
| `.SendJSON(v)`                                             | Explicitly JSON-encode v and set `Content-Type: application/json` |
| `.SendForm(url.Values)`                                    | Send URL-encoded form data                                        |
| `.Query(key, value)`                                       | Append a query parameter                                          |

## Assertions

All assertion methods return the request for chaining. Assertions run when `.Test(t)` is called.

| Method                                 | Checks                                                    | Notes                                                                                             |
|----------------------------------------|-----------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| `.Expect(status)`                      | HTTP status code                                          |                                                                                                   |
| `.ExpectHeader(key, substr)`           | Header contains substring (key is case-insensitive)       |                                                                                                   |
| `.ExpectHeaderExact(key, value)`       | Header exact match                                        |                                                                                                   |
| `.ExpectBody(expected)`                | JSON subset if valid JSON, trimmed exact string otherwise |                                                                                                   |
| `.ExpectBodyExact(expected)`           | Trimmed exact string match                                |                                                                                                   |
| `.ExpectBodyContains(substr)`          | Body contains substring                                   |                                                                                                   |
| `.ExpectBodyMatchesJSON(v)`            | Deep equality after JSON round-trip                       |                                                                                                   |
| `.ExpectBodyArrayContains(path, v)`    | JSON array at `path` contains an element matching `v`     | Use `""` for top-level array responses, or pass a dot path such as `"foo.bar"` for nested arrays. |
| `.ExpectBodyContainsJSON(path, value)` | Dot-path traversal, e.g. `"users.0.name"`                 |                                                                                                   |
| `.ExpectMatchesSpec(spec)`             | Response matches the OpenAPI operation for the request    | Load the spec once with `MustOpenAPISpec` or `LoadOpenAPISpec` and reuse it across requests.      |
| `.ExpectFn(func(*Response) error)`     | Custom assertion                                          |                                                                                                   |

`.Test(t)` executes the request, runs all assertions, and returns `*Response` for further inspection.

### OpenAPI assertions

Load a spec once and reuse it across requests:

```go
spec := supergo.MustOpenAPISpec("example/openapi.yaml")

supergo.New(handler).
    Get("/books/1").
    Expect(200).
    ExpectMatchesSpec(spec).
    Test(t)
```

`ExpectMatchesSpec` infers the OpenAPI operation from the request method and path,
including templated paths like `/books/{id}`, then validates the response status,
content type, and JSON body shape against the declared response schema.


## Agent history

`NewAgent` records every request/response cycle.

```go
agent := supergo.NewAgent(handler)
agent.Post("/login").Expect(200).Test(t)
agent.Get("/profile").Expect(200).Test(t)

history := agent.History()
// history[0].Method, history[0].Path, history[0].Response.StatusCode, history[0].Assertions
agent.ClearHistory()
```

## Stubbing external HTTP dependencies

When your handler calls out to an external service, use `NewStub` to stand in for it during tests. The stub runs a real TCP server so your handler's HTTP client connects normally.

```go
stub := supergo.NewStub(t).
    On("GET", "/cover").
    RespondJSON(200, map[string]string{"url": "https://covers.example.com/dune.jpg"})

client := supergo.NewOutboundHTTPClient(t, stub.URL)

// Pass config into the handler so it calls the stub instead of the real service.
handler := NewRouterWithConfig(store, Config{
    CoverServiceURL: stub.URL,
    HTTPClient:      client,
})
```

### Stub methods

| Method                        | Purpose                                                       | Notes                                                                                                    |
|-------------------------------|---------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| `supergo.NewStub(t)`          | Create a real TCP stub server for outbound HTTP dependencies  | The server closes automatically via `t.Cleanup`; use `stub.URL` as the dependency base URL.              |
| `.Strict()`                   | Fail immediately on any unregistered request                  | Unregistered routes otherwise return `404` by default. Call before `On(...)`.                            |
| `.On(method, path)`           | Register one stubbed route and start configuring its response | Returns a `*StubRoute`; `method` should be uppercase and `path` should start with `/`.                   |
| `.MustBeCalled()`             | Assert that a registered route was hit at least once          | Registers a teardown check; chain it before `Respond*`.                                                  |
| `.Respond(status, body)`      | Return a fixed status and raw byte body                       | `body` may be `nil` for an empty response.                                                               |
| `.RespondJSON(status, v)`     | Return JSON with `Content-Type: application/json`             | `v` can be a static value or `func(*http.Request) any` for per-request dynamic responses.                |
| `.RespondFn(fn)`              | Return a fully custom response                                | Full control over headers, status, and body; request capture still happens automatically.                |
| `.ThenRespond(status, body)`  | Append the next raw response in a sequence                    | Available on the `*StubSequence` returned by `Respond*`; the last response repeats for later calls.      |
| `.ThenRespondJSON(status, v)` | Append the next JSON response in a sequence                   | Supports the same static or dynamic `v` forms as `RespondJSON`.                                          |
| `.ThenRespondFn(fn)`          | Append the next fully custom response in a sequence           | Use when later calls need custom branching or headers.                                                   |
| `.Received(method, path)`     | Inspect captured requests for a route                         | Returns requests in arrival order; each entry exposes `Query()`, `Header`, `Body`, `Method`, and `Path`. |

The server closes automatically via `t.Cleanup`.

When you want to block accidental outbound HTTP in tests, inject an `HTTPClient`
that only allows known destinations. In the example above, `supergo.NewOutboundHTTPClient`
permits the stub URL; you can also pass additional known external base URLs for
services you intentionally do not stub, such as a shared cache or database proxy.

### Response types

```go
// Static JSON (marshalled once at registration time)
.RespondJSON(200, map[string]string{"status": "ok"})

// Dynamic JSON: derive the response from the incoming request
.RespondJSON(200, func(r *http.Request) any {
    return map[string]string{"echo": r.URL.Query().Get("q")}
})

// Plain bytes
.Respond(200, []byte("hello"))

// Full handler control
.RespondFn(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(202)
    fmt.Fprintln(w, "custom")
})
```

### Response sequences

Use `Then*` to return different responses on successive calls. The last response in the sequence repeats for any additional calls.

```go
stub := supergo.NewStub(t).
    On("GET", "/inventory").
    RespondJSON(200, map[string]int{"stock": 5}).  // 1st call
    ThenRespondJSON(200, map[string]int{"stock": 0}) // 2nd+ calls
```

`ThenRespond` and `ThenRespondFn` are also available.

### Inspecting received requests

```go
reqs := stub.Received("GET", "/cover")

len(reqs)                        // number of times the route was called
reqs[0].Query().Get("title")     // parsed query parameter
reqs[0].Header.Get("X-Api-Key")  // request header
reqs[0].Body                     // raw request body ([]byte)
reqs[0].Method                   // "GET"
reqs[0].Path                     // "/cover"
```

### Guards

**`MustBeCalled()`**: fails the test at teardown if the route was never hit:

```go
stub.On("GET", "/cover").
	MustBeCalled().
	RespondJSON(200, data)
```

**`Strict()`**: fails the test immediately if any unregistered route is called:

```go
stub := supergo.NewStub(t).
	Strict().
    On("GET", "/cover").
	RespondJSON(200, data)
```

Both guards can be combined:

```go
stub := supergo.NewStub(t).Strict().
    On("GET", "/cover").
	MustBeCalled().
	RespondJSON(200, data)
```

## Full example

```go
func TestCreateBook(t *testing.T) {
    stub := supergo.NewStub(t).
		Strict().
        On("GET", "/cover").
        MustBeCalled().
        RespondJSON(200, func(r *http.Request) any {
            title := r.URL.Query().Get("title")
            return map[string]string{"url": "https://covers.example.com/" + url.QueryEscape(title) + ".jpg"}
        })

    store := NewStore()
    agent := supergo.NewAgent(NewRouterWithConfig(store, Config{
        CoverServiceURL: stub.URL,
        HTTPClient:      supergo.NewOutboundHTTPClient(t, stub.URL),
    }))

    agent.Post("/login").
        SendJSON(map[string]string{"username": "admin", "password": "secret"}).
        Expect(200).
        Test(t)

    agent.Post("/books").
        SendJSON(Book{Title: "Dune", Author: "Herbert"}).
        Expect(201).
        ExpectBodyContainsJSON("title", "Dune").
        ExpectBodyContains("covers.example.com").
        Test(t)

    if stub.Received("GET", "/cover")[0].Query().Get("title") != "Dune" {
        t.Error("cover service called with wrong title")
    }
}
```

## Development

### Setup

This repo uses Go `1.26.2` and includes a [mise](https://mise.jdx.dev/) config in [`mise.toml`](./mise.toml).

```bash
mise install
mise exec -- go version
```

If you do not use `mise`, install Go `1.26.2` manually and ensure `go` is on your `PATH`.

### Test

Run the full test suite:

```bash
go test ./...
```

Run the same race-enabled test command used in CI:

```bash
go test ./... -race
```

### Lint

Run `golangci-lint` with the repo config:

```bash
golangci-lint run
```
