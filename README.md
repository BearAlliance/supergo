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

| Method                        | Purpose                                                       | Notes                                                                                                     |
|-------------------------------|---------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| `supergo.NewStub(t)`          | Create a real TCP stub server for outbound HTTP dependencies  | The server closes automatically via `t.Cleanup`; use `stub.URL` as the dependency base URL.               |
| `.Strict()`                   | Fail immediately on any unregistered request                  | Unregistered routes otherwise return `404` by default. Call before `On(...)`.                             |
| `.MustAllBeCalled()`          | Assert that every registered route was hit at least once      | Registers one teardown check for the whole stub; use it when all routes on the stub are expected to fire. |
| `.On(method, path)`           | Register one stubbed route and start configuring its response | Returns a `*StubRoute`; `method` should be uppercase and `path` should start with `/`.                    |
| `.MustBeCalled()`             | Assert that a registered route was hit at least once          | Registers a teardown check; chain it before `Respond*`.                                                   |
| `.MustBeCalledTimes(n)`       | Assert that a registered route was hit exactly `n` times      | Registers a teardown check; chain it before `Respond*`. Subsumes `MustBeCalled()` when `n >= 1`.          |
| `.Respond(status, body)`      | Return a fixed status and raw byte body                       | `body` may be `nil` for an empty response.                                                                |
| `.RespondJSON(status, v)`     | Return JSON with `Content-Type: application/json`             | `v` can be a static value or `func(*http.Request) any` for per-request dynamic responses.                 |
| `.RespondFn(fn)`              | Return a fully custom response                                | Full control over headers, status, and body; request capture still happens automatically.                 |
| `.ThenRespond(status, body)`  | Append the next raw response in a sequence                    | Available on the `*StubSequence` returned by `Respond*`; the last response repeats for later calls.       |
| `.ThenRespondJSON(status, v)` | Append the next JSON response in a sequence                   | Supports the same static or dynamic `v` forms as `RespondJSON`.                                           |
| `.ThenRespondFn(fn)`          | Append the next fully custom response in a sequence           | Use when later calls need custom branching or headers.                                                    |
| `.Received(method, path)`     | Inspect captured requests for a route                         | Returns requests in arrival order; each entry exposes `Query()`, `Header`, `Body`, `Method`, and `Path`.  |

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

**`MustBeCalledTimes(n)`**: fails the test at teardown if the route was not called exactly `n` times:

```go
stub.On("GET", "/cover").
	MustBeCalledTimes(2).
	RespondJSON(200, data)
```

**`MustAllBeCalled()`**: fails the test at teardown if any registered route on the stub was never hit:

```go
stub := supergo.NewStub(t).
	MustAllBeCalled().
	On("GET", "/cover").
	RespondJSON(200, coverData).
	On("POST", "/audit").
	RespondJSON(202, auditData)
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
stub := supergo.NewStub(t).Strict().MustAllBeCalled().
    On("GET", "/cover").
	RespondJSON(200, data)
```

## Faking stateful external dependencies

A stub returns canned responses per route. A **fake** goes one rung further: you
give `NewFake` a real in-memory `http.Handler`, so a `POST` mutates state that a
later `GET` reflects. Use a fake when the dependency's behavior, not just a fixed
reply, matters to the test.

You could host such a handler yourself with `httptest`; the reason to route it
through supergo is what it wraps around your behavior:

- **`VerifySpec` checks the fake against the real service's OpenAPI spec on every
  call**, so the fake cannot silently drift from the contract it stands in for.
  This is the failure mode hand-rolled fakes never catch: the fake keeps
  returning a shape the real service stopped returning, tests stay green, prod
  breaks.
- The same request capture and interaction guards as stubs (`Received`,
  `MustBeCalled`, `MustBeCalledTimes`) come for free, so the fake is also a spy.

```go
// coverService() returns your in-memory http.Handler with real behavior.
fake := supergo.NewFake(t, coverService()).
    VerifySpec(spec).                 // every response validated against the spec
    MustBeCalled("GET", "/cover")     // interaction guard, checked at teardown

handler := NewRouterWithConfig(store, Config{
    CoverServiceURL: fake.URL,
    HTTPClient:      supergo.NewOutboundHTTPClient(t, fake.URL),
})

// After the test runs, inspect what the handler sent:
reqs := fake.Received("GET", "/cover")
```

### Fake methods

| Method                                | Purpose                                                          | Notes                                                                                           |
|---------------------------------------|------------------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `supergo.NewFake(t, handler)`         | Create a real TCP server backed by your in-memory `http.Handler` | Closes automatically via `t.Cleanup`; use `fake.URL` as the dependency base URL.                |
| `.VerifySpec(spec)`                   | Validate every response against the OpenAPI spec for its route   | Fails the test on schema/status/content-type drift, or on an operation the spec never declares. |
| `.MustBeCalled(method, path)`         | Assert the route was hit at least once                           | Registers a teardown check. `path` matches the concrete request path, not a template.           |
| `.MustBeCalledTimes(method, path, n)` | Assert the route was hit exactly `n` times                       | Registers a teardown check.                                                                     |
| `.Received(method, path)`             | Inspect captured requests for a route                            | Same `CapturedRequest` values (`Query()`, `Header`, `Body`, `Method`, `Path`) as stubs.         |

Load the spec once with `MustOpenAPISpec` or `LoadOpenAPISpec` and reuse it, the same way you do for `ExpectMatchesSpec`.

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
