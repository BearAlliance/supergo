// Package example demonstrates a small Gin API used to test the supergo library.
package example

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

// Book is the domain model.
type Book struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

// Store is an in-memory book store.
type Store struct {
	mu     sync.RWMutex
	books  map[int]Book
	nextID int
}

func NewStore() *Store {
	return &Store{
		books:  make(map[int]Book),
		nextID: 1,
	}
}

func (s *Store) Add(b Book) Book {
	s.mu.Lock()
	defer s.mu.Unlock()
	b.ID = s.nextID
	s.nextID++
	s.books[b.ID] = b
	return b
}

func (s *Store) All() []Book {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Book, 0, len(s.books))
	for _, b := range s.books {
		out = append(out, b)
	}
	return out
}

func (s *Store) Get(id int) (Book, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.books[id]
	return b, ok
}

func (s *Store) Delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.books[id]; !ok {
		return false
	}
	delete(s.books, id)
	return true
}

// sessions is a trivial in-memory session store (token → username).
var sessions = &sessionStore{tokens: make(map[string]string)}

type sessionStore struct {
	mu     sync.RWMutex
	tokens map[string]string
}

func (ss *sessionStore) set(token, user string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.tokens[token] = user
}

func (ss *sessionStore) get(token string) (string, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	u, ok := ss.tokens[token]
	return u, ok
}

func (ss *sessionStore) delete(token string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.tokens, token)
}

// NewRouter builds and returns the Gin engine with all routes registered.
// Accepts a *Store so tests can inject a fresh one each time.
func NewRouter(store *Store) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// Auth routes
	r.POST("/login", func(c *gin.Context) {
		var creds struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&creds); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		if creds.Username != "admin" || creds.Password != "secret" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		token := "tok-" + creds.Username
		sessions.set(token, creds.Username)
		http.SetCookie(c.Writer, &http.Cookie{
			Name:  "session",
			Value: token,
		})
		c.JSON(http.StatusOK, gin.H{"token": token})
	})

	r.POST("/logout", func(c *gin.Context) {
		cookie, err := c.Request.Cookie("session")
		if err == nil {
			sessions.delete(cookie.Value)
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: "session", MaxAge: -1})
		c.Status(http.StatusNoContent)
	})

	// authMiddleware rejects requests that don't carry a valid session cookie.
	authMiddleware := func(c *gin.Context) {
		cookie, err := c.Request.Cookie("session")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		user, ok := sessions.get(cookie.Value)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			return
		}
		c.Set("user", user)
		c.Next()
	}

	books := r.Group("/books")

	// Public: list books (supports ?author= filter)
	books.GET("", func(c *gin.Context) {
		all := store.All()
		if author := c.Query("author"); author != "" {
			filtered := all[:0]
			for _, b := range all {
				if b.Author == author {
					filtered = append(filtered, b)
				}
			}
			all = filtered
		}
		c.JSON(http.StatusOK, all)
	})

	// Public: get single book
	books.GET("/:id", func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		book, ok := store.Get(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, book)
	})

	// Protected: create book
	books.POST("", authMiddleware, func(c *gin.Context) {
		var b Book
		if err := c.ShouldBindJSON(&b); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
			return
		}
		created := store.Add(b)
		c.JSON(http.StatusCreated, created)
	})

	// Protected: delete book
	books.DELETE("/:id", authMiddleware, func(c *gin.Context) {
		id, err := strconv.Atoi(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if !store.Delete(id) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Status(http.StatusNoContent)
	})

	return r
}
