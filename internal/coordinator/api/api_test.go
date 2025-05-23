package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pkg/errors"
	"github.com/runabol/tork"
	"github.com/runabol/tork/datastore"
	"github.com/runabol/tork/datastore/postgres"
	"github.com/runabol/tork/middleware/web"

	"github.com/runabol/tork/broker"

	"github.com/runabol/tork/internal/uuid"
	"github.com/stretchr/testify/assert"
)

func Test_getQueues(t *testing.T) {
	b := broker.NewInMemoryBroker()
	err := b.SubscribeForTasks("some-queue", func(t *tork.Task) error {
		return nil
	})
	assert.NoError(t, err)
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/queues", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	qs := []broker.QueueInfo{}
	err = json.Unmarshal(body, &qs)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(qs))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_listJobs(t *testing.T) {
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)

	for i := 0; i < 101; i++ {
		err := ds.CreateJob(context.Background(), &tork.Job{
			ID: uuid.NewUUID(),
		})
		assert.NoError(t, err)
	}

	req, err := http.NewRequest("GET", "/jobs", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	js := datastore.Page[*tork.Job]{}
	err = json.Unmarshal(body, &js)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, 10, js.Size)
	assert.Equal(t, 11, js.TotalPages)
	assert.Equal(t, 1, js.Number)
	assert.Equal(t, http.StatusOK, w.Code)

	req, err = http.NewRequest("GET", "/jobs?page=11", nil)
	assert.NoError(t, err)
	w = httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err = io.ReadAll(w.Body)
	assert.NoError(t, err)

	js = datastore.Page[*tork.Job]{}
	err = json.Unmarshal(body, &js)
	assert.NoError(t, err)

	assert.Equal(t, 1, js.Size)
	assert.Equal(t, 11, js.TotalPages)
	assert.Equal(t, 11, js.Number)
	assert.Equal(t, http.StatusOK, w.Code)

	req, err = http.NewRequest("GET", "/jobs?page=1&size=50", nil)
	assert.NoError(t, err)
	w = httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err = io.ReadAll(w.Body)
	assert.NoError(t, err)

	js = datastore.Page[*tork.Job]{}
	err = json.Unmarshal(body, &js)
	assert.NoError(t, err)

	assert.Equal(t, 20, js.Size)
	assert.Equal(t, 6, js.TotalPages)
	assert.Equal(t, 1, js.Number)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_getActiveNodes(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	active := &tork.Node{
		ID:              "1234",
		LastHeartbeatAt: time.Now().UTC(),
	}
	inactive := &tork.Node{
		ID:              "2345",
		LastHeartbeatAt: time.Now().UTC().Add(-time.Hour),
	}
	err = ds.CreateNode(context.Background(), active)
	assert.NoError(t, err)
	err = ds.CreateNode(context.Background(), inactive)
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/nodes", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	nrs := []tork.Node{}
	err = json.Unmarshal(body, &nrs)
	assert.NoError(t, err)

	assert.Equal(t, 1, len(nrs))
	assert.Equal(t, "1234", nrs[0].ID)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_healthOK(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)

	assert.NoError(t, err)
	assert.Contains(t, string(body), "\"status\":\"UP\"")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_healthNotOK(t *testing.T) {
	schemaName := fmt.Sprintf("tork%d", rand.Int())
	dsn := `host=localhost user=tork password=tork dbname=tork search_path=%s sslmode=disable`
	ds, err := postgres.NewPostgresDataStore(fmt.Sprintf(dsn, schemaName))
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	err = ds.Close()
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)

	assert.NoError(t, err)
	assert.Contains(t, string(body), "\"status\":\"DOWN\"")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_getUnknownTask(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/task/1", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	_, err = io.ReadAll(w.Body)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_getTask(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()
	u := &tork.User{
		ID:        uuid.NewUUID(),
		Username:  uuid.NewShortUUID(),
		Name:      "Tester",
		CreatedAt: &now,
	}
	err = ds.CreateUser(ctx, u)
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		CreatedBy: u,
		Tags:      []string{"tag-a", "tag-b"},
		AutoDelete: &tork.AutoDelete{
			After: "5h",
		},
		Secrets: map[string]string{
			"password": "secret",
		},
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)
	ta := tork.Task{
		ID:        "1234",
		Name:      "test task",
		CreatedAt: &now,
		JobID:     j1.ID,
	}
	err = ds.CreateTask(ctx, &ta)
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/tasks/1234", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)
	tr := tork.Task{}
	err = json.Unmarshal(body, &tr)
	assert.NoError(t, err)
	assert.Equal(t, "1234", tr.ID)
	assert.Equal(t, "test task", tr.Name)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_createJob(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("POST", "/jobs", strings.NewReader(`{
		"name":"test job",
		"tasks":[{
			"name":"test task",
			"image":"some:image"
		}]
	}`))
	req.Header.Add("Content-Type", "application/json")
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)
	j := tork.Job{}
	err = json.Unmarshal(body, &j)
	assert.NoError(t, err)
	assert.Equal(t, tork.JobStatePending, j.State)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_createJobInvalidProperty(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("POST", "/jobs", strings.NewReader(`{
		"tasks":[{
			"nosuch":"thing",
			"image":"some:image"
		}]
	}`))
	req.Header.Add("Content-Type", "application/json")
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_getJob(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	err = ds.CreateJob(context.Background(), &tork.Job{
		ID:    "1234",
		State: tork.JobStatePending,
	})
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/jobs/1234", nil)
	req.Header.Add("Content-Type", "application/json")
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)
	j := tork.Job{}
	err = json.Unmarshal(body, &j)
	assert.NoError(t, err)
	assert.Equal(t, "1234", j.ID)
	assert.Equal(t, tork.JobStatePending, j.State)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_cancelRunningJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		State:     tork.JobStateRunning,
		CreatedAt: time.Now().UTC(),
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)

	now := time.Now().UTC()

	tasks := []tork.Task{{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStatePending,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateScheduled,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateRunning,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateCancelled,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateCompleted,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateFailed,
		CreatedAt: &now,
		JobID:     j1.ID,
	}}

	for _, ta := range tasks {
		err := ds.CreateTask(ctx, &ta)
		assert.NoError(t, err)
	}

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/cancel", j1.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, "{\"status\":\"OK\"}\n", string(body))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_cancelScheduledJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		State:     tork.JobStateScheduled,
		CreatedAt: time.Now().UTC(),
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)

	now := time.Now().UTC()

	tasks := []tork.Task{{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStatePending,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateScheduled,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateRunning,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateCancelled,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateCompleted,
		CreatedAt: &now,
		JobID:     j1.ID,
	}, {
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateFailed,
		CreatedAt: &now,
		JobID:     j1.ID,
	}}

	for _, ta := range tasks {
		err := ds.CreateTask(ctx, &ta)
		assert.NoError(t, err)
	}

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/cancel", j1.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, "{\"status\":\"OK\"}\n", string(body))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_restartJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		State:     tork.JobStateCancelled,
		CreatedAt: time.Now().UTC(),
		Position:  1,
		Tasks: []*tork.Task{
			{
				Name: "some fake task",
			},
		},
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)

	now := time.Now().UTC()

	tasks := []tork.Task{{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStatePending,
		CreatedAt: &now,
		JobID:     j1.ID,
	}}

	for _, ta := range tasks {
		err := ds.CreateTask(ctx, &ta)
		assert.NoError(t, err)
	}

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/restart", j1.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, "{\"status\":\"OK\"}\n", string(body))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_restartRunningJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		State:     tork.JobStateRunning,
		CreatedAt: time.Now().UTC(),
		Position:  1,
		Tasks: []*tork.Task{
			{
				Name: "some fake task",
			},
		},
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)

	now := time.Now().UTC()

	tasks := []tork.Task{{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStatePending,
		CreatedAt: &now,
		JobID:     j1.ID,
	}}

	for _, ta := range tasks {
		err := ds.CreateTask(ctx, &ta)
		assert.NoError(t, err)
	}

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/restart", j1.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_restartRunningNoMoreTasksJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	j1 := tork.Job{
		ID:        uuid.NewUUID(),
		State:     tork.JobStateFailed,
		CreatedAt: time.Now().UTC(),
		Position:  2,
		Tasks: []*tork.Task{
			{
				Name: "some fake task",
			},
		},
	}
	err = ds.CreateJob(ctx, &j1)
	assert.NoError(t, err)

	now := time.Now().UTC()

	tasks := []tork.Task{{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStatePending,
		CreatedAt: &now,
		JobID:     j1.ID,
	}}

	for _, ta := range tasks {
		err := ds.CreateTask(ctx, &ta)
		assert.NoError(t, err)
	}

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("PUT", fmt.Sprintf("/jobs/%s/restart", j1.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_middleware(t *testing.T) {
	mw := func(next web.HandlerFunc) web.HandlerFunc {
		return func(c web.Context) error {
			if strings.HasPrefix(c.Request().URL.Path, "/middleware") {
				return c.String(http.StatusOK, "OK")
			}
			return next(c)
		}
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Middleware: Middleware{
			Web: []web.MiddlewareFunc{mw},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/middleware", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", string(body))
	assert.NoError(t, ds.Close())
}

func Test_echoMiddleware(t *testing.T) {
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Middleware: Middleware{
			Echo: []echo.MiddlewareFunc{
				func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c echo.Context) error {
						return c.String(http.StatusOK, "OK")
					}
				},
			},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/middleware", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", string(body))
	assert.NoError(t, ds.Close())
}

func Test_middlewareMultiple(t *testing.T) {
	mw1 := func(next web.HandlerFunc) web.HandlerFunc {
		return func(c web.Context) error {
			if strings.HasPrefix(c.Request().URL.Path, "/middleware1") {
				return c.String(http.StatusOK, "OK1")
			}
			return next(c)
		}
	}
	mw2 := func(next web.HandlerFunc) web.HandlerFunc {
		return func(c web.Context) error {
			if strings.HasPrefix(c.Request().URL.Path, "/middleware2") {
				return c.String(http.StatusOK, "OK2")
			}
			return next(c)
		}
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Middleware: Middleware{
			Web: []web.MiddlewareFunc{mw1, mw2},
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)

	req, err := http.NewRequest("GET", "/middleware1", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK1", string(body))

	req, err = http.NewRequest("GET", "/middleware2", nil)
	assert.NoError(t, err)
	w = httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err = io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK2", string(body))
	assert.NoError(t, ds.Close())
}

func Test_2CustomEndpoints(t *testing.T) {
	h := func(c web.Context) error {
		return c.String(http.StatusOK, "OK")
	}
	h2 := func(c web.Context) error {
		return c.String(http.StatusOK, "OK 2")
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Endpoints: map[string]web.HandlerFunc{
			"GET /myendpoint":       h,
			"POST /myotherendpoint": h2,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/myendpoint", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", string(body))

	req, err = http.NewRequest("POST", "/myotherendpoint", nil)
	assert.NoError(t, err)
	api.server.Handler.ServeHTTP(w, req)
	body, err = io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK 2", string(body))
	assert.NoError(t, ds.Close())
}

func Test_customEndpoint(t *testing.T) {
	h := func(c web.Context) error {
		return c.String(http.StatusOK, "OK")
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Endpoints: map[string]web.HandlerFunc{
			"GET /myendpoint": h,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/myendpoint", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", string(body))
	assert.NoError(t, ds.Close())
}

func Test_customEndpointInvalidSpec(t *testing.T) {
	h := func(c web.Context) error {
		return c.String(http.StatusOK, "OK")
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	_, err = NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Endpoints: map[string]web.HandlerFunc{
			"xyz": h,
		},
	})
	assert.Error(t, err)
	assert.NoError(t, ds.Close())
}

func Test_customEndpointError(t *testing.T) {
	h := func(c web.Context) error {
		c.Error(http.StatusBadRequest, errors.Errorf("bad stuff happened"))
		return nil
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Endpoints: map[string]web.HandlerFunc{
			"GET /myendpoint": h,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/myendpoint", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, `{"message":"bad stuff happened"}`, strings.TrimSpace(string(body)))
	assert.NoError(t, ds.Close())
}

func Test_customEndpointBind(t *testing.T) {
	h := func(c web.Context) error {
		type MyStruct struct {
			Name string `json:"name"`
		}
		s := MyStruct{}
		if err := c.Bind(&s); err != nil {
			return err
		}
		return c.String(http.StatusOK, s.Name)
	}
	b := broker.NewInMemoryBroker()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    b,
		Endpoints: map[string]web.HandlerFunc{
			"POST /myendpoint": h,
		},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("POST", "/myendpoint", strings.NewReader(`{"name":"Me"}`))
	req.Header.Add("content-type", "application/json")
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Me", string(body))
	assert.NoError(t, ds.Close())
}

func Test_disableEndpoint(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
		Enabled:   map[string]bool{"health": false},
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	_, err = io.ReadAll(w.Body)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NoError(t, ds.Close())
}

func TestShutdown(t *testing.T) {
	mw := func(next web.HandlerFunc) web.HandlerFunc {
		return func(c web.Context) error {
			select {
			case <-time.After(time.Hour):
			case <-c.Done():
			}
			return next(c)
		}
	}

	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
		Middleware: Middleware{
			Web: []web.MiddlewareFunc{mw},
		},
	})
	assert.NoError(t, err)

	go func() {
		time.Sleep(time.Second)
		assert.NoError(t, api.Shutdown(context.Background()))
	}()

	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("GET", "/middleware", nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	assert.NoError(t, ds.Close())
}

func Test_createJobAndWait(t *testing.T) {
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)
	req, err := http.NewRequest("POST", "/jobs", strings.NewReader(`{
		"name":"test job",
		"wait": {
			"timeout": "1s"
		},
		"tasks":[{
			"name":"test task",
			"image":"some:image"
		}]
	}`))
	req.Header.Add("Content-Type", "application/json")
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestTimeout, w.Code)
	assert.NoError(t, ds.Close())
}

func Test_deleteScheduledJob(t *testing.T) {
	ctx := context.Background()
	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)

	// Create a scheduled job
	sj := tork.ScheduledJob{
		ID:        uuid.NewUUID(),
		State:     tork.ScheduledJobStateActive,
		CreatedAt: time.Now().UTC(),
	}
	err = ds.CreateScheduledJob(ctx, &sj)
	assert.NoError(t, err)

	api, err := NewAPI(Config{
		DataStore: ds,
		Broker:    broker.NewInMemoryBroker(),
	})
	assert.NoError(t, err)
	assert.NotNil(t, api)

	// Send DELETE request to delete the scheduled job
	req, err := http.NewRequest("DELETE", fmt.Sprintf("/scheduled-jobs/%s", sj.ID), nil)
	assert.NoError(t, err)
	w := httptest.NewRecorder()
	api.server.Handler.ServeHTTP(w, req)
	body, err := io.ReadAll(w.Body)
	assert.NoError(t, err)

	// Verify response
	assert.Equal(t, "{\"status\":\"OK\"}\n", string(body))
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify the job state is updated to paused
	_, err = ds.GetScheduledJobByID(ctx, sj.ID)
	assert.Error(t, err)

	assert.NoError(t, ds.Close())
}
