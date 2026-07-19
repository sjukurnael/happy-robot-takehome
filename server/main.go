package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	ctx := context.Background()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://app:app@localhost:5432/taskman?sslmode=disable"
	}
	pool, err := connectDB(ctx, databaseURL)
	if err != nil {
		log.Fatal("failed to connect to database: ", err)
	}
	defer pool.Close()

	store := NewStore(pool)
	hub := NewHub(store)
	ai := newAIClient()
	if !ai.enabled {
		log.Println("AI breakdown disabled: ANTHROPIC_API_KEY not set")
	}
	api := &API{store: store, hub: hub, ai: ai}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)
	r.Use(newRateLimiter().middleware)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Get("/ws", hub.ServeWS)

	r.Get("/api/docs", serveDocs)
	r.Get("/api/openapi.yaml", serveOpenAPISpec)

	r.Route("/api/projects", func(r chi.Router) {
		r.Get("/", api.listProjects)
		r.Post("/", api.createProject)
		r.Get("/stats", api.listProjectStats)
		r.Route("/{projectID}", func(r chi.Router) {
			r.Get("/", api.getProject)
			r.Patch("/", api.updateProject)
			r.Delete("/", api.deleteProject)
			r.Get("/tasks", api.listTasks)
			r.Post("/tasks", api.createTask)
			r.Get("/events", api.listEvents)
		})
	})

	r.Route("/api/tasks/{taskID}", func(r chi.Router) {
		r.Get("/", api.getTask)
		r.Patch("/", api.updateTask)
		r.Delete("/", api.deleteTask)
		r.Get("/comments", api.listComments)
		r.Post("/comments", api.createComment)
		r.Post("/breakdown", api.breakdownTask)
		r.Post("/breakdown/apply", api.applyBreakdown)
	})

	r.Delete("/api/comments/{commentID}", api.deleteComment)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("server listening on :" + port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Actor")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
