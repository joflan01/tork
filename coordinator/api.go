package coordinator

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/tork/datastore"
	"github.com/tork/mq"
	"github.com/tork/task"
	"github.com/tork/uuid"
)

type api struct {
	server *http.Server
	broker mq.Broker
	ds     datastore.TaskDatastore
}

func newAPI(cfg Config) *api {
	if cfg.Address == "" {
		cfg.Address = ":3000"
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(errorHandler)
	s := &api{
		broker: cfg.Broker,
		server: &http.Server{
			Addr:    cfg.Address,
			Handler: r,
		},
		ds: cfg.TaskDataStore,
	}
	r.GET("/status", s.status)
	r.POST("/task", s.createTask)
	r.GET("/task/:id", s.getTask)
	r.GET("/queue", s.listQueues)
	return s
}

func errorHandler(c *gin.Context) {
	c.Next()
	if len(c.Errors) > 0 {
		c.JSON(-1, c.Errors[0])
	}
}

func (s *api) status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "OK"})
}

func (s *api) listQueues(c *gin.Context) {
	qs, err := s.broker.Queues(c)
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, qs)
}

func (s *api) createTask(c *gin.Context) {
	t := &task.Task{}
	if err := c.BindJSON(&t); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(t.Image) == "" {
		c.AbortWithError(http.StatusBadRequest, errors.New("missing required field: image"))
		return
	}
	t.ID = uuid.NewUUID()
	t.State = task.Pending
	t.CreatedAt = time.Now()
	if err := s.ds.Save(c, t); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}
	log.Info().Any("task", t).Msg("received task")
	if err := s.broker.Publish(c, mq.QUEUE_PENDING, t); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (s *api) getTask(c *gin.Context) {
	id := c.Param("id")
	t, err := s.ds.GetByID(c, id)
	if err != nil {
		c.AbortWithError(http.StatusNotFound, err)
		return
	}
	c.JSON(http.StatusOK, t)
}

func (s *api) start() error {
	go func() {
		// service connections
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msgf("error starting up server")
		}
	}()
	return nil
}

func (s *api) shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}