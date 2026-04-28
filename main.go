// demo-api — A minimal task management API with an embedded OpenAPI spec.
//
// Endpoints:
//   GET    /api/tasks          List all tasks
//   POST   /api/tasks          Create a task
//   GET    /api/tasks/{id}     Get a task by ID
//   PATCH  /api/tasks/{id}     Update a task
//   DELETE /api/tasks/{id}     Delete a task
//   GET    /openapi.json       Serve the OpenAPI 3.1 spec
//
// Run:
//   go run main.go
//   curl http://localhost:8080/openapi.json
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// --- Models ---

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"` // "pending", "in_progress", "done"
	Priority    int        `json:"priority"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

type CreateTaskRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Priority    int        `json:"priority,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

type UpdateTaskRequest struct {
	Title       *string    `json:"title,omitempty"`
	Description *string    `json:"description,omitempty"`
	Status      *string    `json:"status,omitempty"`
	Priority    *int       `json:"priority,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
}

type TaskListResponse struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Success bool   `json:"success"`
}

// --- In-memory store ---

type store struct {
	mu    sync.RWMutex
	tasks map[string]Task
}

func newStore() *store {
	return &store{tasks: make(map[string]Task)}
}

// --- Handlers ---

func (s *store) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listTasks(w, r)
	case http.MethodPost:
		s.createTask(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *store) handleTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing task id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getTask(w, id)
	case http.MethodPatch:
		s.updateTask(w, r, id)
	case http.MethodDelete:
		s.deleteTask(w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *store) listTasks(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tasks := make([]Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		tasks = append(tasks, t)
	}
	writeJSON(w, http.StatusOK, TaskListResponse{Tasks: tasks, Total: len(tasks)})
}

func (s *store) createTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusUnprocessableEntity, "title is required")
		return
	}
	now := time.Now().UTC()
	task := Task{
		ID:          uuid.New().String(),
		Title:       req.Title,
		Description: req.Description,
		Status:      "pending",
		Priority:    req.Priority,
		CreatedAt:   now,
		UpdatedAt:   now,
		DueDate:     req.DueDate,
	}
	s.mu.Lock()
	s.tasks[task.ID] = task
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, task)
}

func (s *store) getTask(w http.ResponseWriter, id string) {
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *store) updateTask(w http.ResponseWriter, r *http.Request, id string) {
	var req UpdateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasks[id]
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if req.Title != nil {
		task.Title = *req.Title
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Status != nil {
		switch *req.Status {
		case "pending", "in_progress", "done":
			task.Status = *req.Status
		default:
			writeError(w, http.StatusUnprocessableEntity, "status must be pending, in_progress, or done")
			return
		}
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.DueDate != nil {
		task.DueDate = req.DueDate
	}
	task.UpdatedAt = time.Now().UTC()
	s.tasks[id] = task
	writeJSON(w, http.StatusOK, task)
}

func (s *store) deleteTask(w http.ResponseWriter, id string) {
	s.mu.Lock()
	_, ok := s.tasks[id]
	if ok {
		delete(s.tasks, id)
	}
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Auth middleware ---

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") == "" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "Forbidden")
			return
		}
		next(w, r)
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg, Success: false})
}

// --- OpenAPI spec (embedded) ---

func serveOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(openapiSpec))
}

const openapiSpec = `{
  "openapi": "3.1.0",
  "info": {
    "title": "Task Manager API",
    "version": "0.1.0",
    "description": "A simple task management REST API for demonstrating SDK generation."
  },
  "servers": [
    { "url": "http://localhost:8080", "description": "Local development" }
  ],
  "security": [
    { "bearerAuth": [] }
  ],
  "paths": {
    "/api/tasks": {
      "get": {
        "operationId": "list_tasks",
        "summary": "List all tasks",
        "parameters": [
          {
            "name": "status",
            "in": "query",
            "schema": { "type": "string", "enum": ["pending", "in_progress", "done"] },
            "description": "Filter by task status"
          },
          {
            "name": "limit",
            "in": "query",
            "schema": { "type": "integer", "default": 50 },
            "description": "Maximum number of tasks to return"
          }
        ],
        "responses": {
          "200": {
            "description": "List of tasks",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/TaskListResponse" }
              }
            }
          },
          "403": { "description": "Forbidden — invalid or missing API key" }
        }
      },
      "post": {
        "operationId": "create_task",
        "summary": "Create a new task",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/CreateTaskRequest" }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Task created",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Task" }
              }
            }
          },
          "400": {
            "description": "Invalid JSON body",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          },
          "422": {
            "description": "Validation error (e.g. missing title)",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      }
    },
    "/api/tasks/{task_id}": {
      "get": {
        "operationId": "get_task",
        "summary": "Get a task by ID",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string", "format": "uuid" },
            "description": "The task UUID"
          }
        ],
        "responses": {
          "200": {
            "description": "Task details",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Task" }
              }
            }
          },
          "404": {
            "description": "Task not found",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      },
      "patch": {
        "operationId": "update_task",
        "summary": "Update a task",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string", "format": "uuid" }
          }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateTaskRequest" }
            }
          }
        },
        "responses": {
          "200": {
            "description": "Updated task",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/Task" }
              }
            }
          },
          "404": {
            "description": "Task not found",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          },
          "422": {
            "description": "Validation error",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      },
      "delete": {
        "operationId": "delete_task",
        "summary": "Delete a task",
        "parameters": [
          {
            "name": "task_id",
            "in": "path",
            "required": true,
            "schema": { "type": "string", "format": "uuid" }
          }
        ],
        "responses": {
          "204": { "description": "Task deleted" },
          "404": {
            "description": "Task not found",
            "content": {
              "application/json": {
                "schema": { "$ref": "#/components/schemas/ErrorResponse" }
              }
            }
          }
        }
      }
    }
  },
  "components": {
    "securitySchemes": {
      "bearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "description": "API key passed as Bearer token"
      }
    },
    "schemas": {
      "Task": {
        "type": "object",
        "required": ["id", "title", "status", "priority", "created_at", "updated_at"],
        "properties": {
          "id":          { "type": "string", "format": "uuid" },
          "title":       { "type": "string" },
          "description": { "type": "string" },
          "status":      { "type": "string", "enum": ["pending", "in_progress", "done"] },
          "priority":    { "type": "integer" },
          "created_at":  { "type": "string", "format": "date-time" },
          "updated_at":  { "type": "string", "format": "date-time" },
          "due_date":    { "type": "string", "format": "date-time", "nullable": true }
        }
      },
      "CreateTaskRequest": {
        "type": "object",
        "required": ["title"],
        "properties": {
          "title":       { "type": "string", "description": "Task title (required)" },
          "description": { "type": "string", "description": "Optional description" },
          "priority":    { "type": "integer", "description": "Priority (0 = default)", "default": 0 },
          "due_date":    { "type": "string", "format": "date-time", "nullable": true }
        }
      },
      "UpdateTaskRequest": {
        "type": "object",
        "properties": {
          "title":       { "type": "string" },
          "description": { "type": "string" },
          "status":      { "type": "string", "enum": ["pending", "in_progress", "done"] },
          "priority":    { "type": "integer" },
          "due_date":    { "type": "string", "format": "date-time", "nullable": true }
        }
      },
      "TaskListResponse": {
        "type": "object",
        "required": ["tasks", "total"],
        "properties": {
          "tasks": { "type": "array", "items": { "$ref": "#/components/schemas/Task" } },
          "total": { "type": "integer" }
        }
      },
      "ErrorResponse": {
        "type": "object",
        "required": ["error", "success"],
        "properties": {
          "error":   { "type": "string", "description": "Human-readable error message" },
          "success": { "type": "boolean", "description": "Always false for errors" }
        }
      }
    }
  }
}`

func main() {
	s := newStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.json", serveOpenAPI)
	mux.HandleFunc("/api/tasks/", authMiddleware(s.handleTask))
	mux.HandleFunc("/api/tasks", authMiddleware(s.handleTasks))

	fmt.Println("Task Manager API listening on :8080")
	fmt.Println("  OpenAPI spec: http://localhost:8080/openapi.json")
	fmt.Println("  API base:     http://localhost:8080/api/tasks")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
