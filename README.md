# supergo

A [supertest](https://www.npmjs.com/package/supertest)-inspired HTTP testing library for Go.
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

| Method                                 | Checks                                                    |
|----------------------------------------|-----------------------------------------------------------|
| `.Expect(status)`                      | HTTP status code                                          |
| `.ExpectHeader(key, substr)`           | Header contains substring (key is case-insensitive)       |
| `.ExpectHeaderExact(key, value)`       | Header exact match                                        |
| `.ExpectBody(expected)`                | JSON subset if valid JSON, trimmed exact string otherwise |
| `.ExpectBodyExact(expected)`           | Trimmed exact string match                                |
| `.ExpectBodyContains(substr)`          | Body contains substring                                   |
| `.ExpectBodyMatchesJSON(v)`            | Deep equality after JSON round-trip                       |
| `.ExpectBodyContainsJSON(path, value)` | Dot-path traversal, e.g. `"users.0.name"`                 |
| `.ExpectFn(func(*Response) error)`     | Custom assertion                                          |

`.Test(t)` executes the request, runs all assertions, and returns `*Response` for further inspection.

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

// Pass stub.URL to the handler so it calls the stub instead of the real service.
handler := NewRouter(store, stub.URL)
```

The server closes automatically via `t.Cleanup`.

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
    agent := supergo.NewAgent(NewRouter(store, stub.URL))

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
