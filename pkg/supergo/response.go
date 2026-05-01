package supergo

import "net/http"

// Response holds the recorded result of a single HTTP exchange.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}
