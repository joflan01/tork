package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/runabol/tork"
	"github.com/runabol/tork/broker"
	"github.com/runabol/tork/datastore/postgres"
	"github.com/runabol/tork/internal/uuid"
	"github.com/runabol/tork/middleware/task"
	"github.com/stretchr/testify/assert"
)

func Test_handleStartedTask(t *testing.T) {
	ctx := context.Background()
	b := broker.NewInMemoryBroker()

	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	handler := NewStartedHandler(ds, b)
	assert.NotNil(t, handler)

	now := time.Now().UTC()

	j1 := &tork.Job{
		ID:    uuid.NewUUID(),
		State: tork.JobStateScheduled,
	}
	err = ds.CreateJob(ctx, j1)
	assert.NoError(t, err)

	t1 := &tork.Task{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateScheduled,
		StartedAt: &now,
		NodeID:    uuid.NewUUID(),
		JobID:     j1.ID,
		CreatedAt: &now,
	}

	err = ds.CreateTask(ctx, t1)
	assert.NoError(t, err)

	err = handler(ctx, task.StateChange, t1)
	assert.NoError(t, err)

	t2, err := ds.GetTaskByID(ctx, t1.ID)
	assert.NoError(t, err)
	assert.Equal(t, tork.TaskStateRunning, t2.State)
	assert.Equal(t, t1.StartedAt.Unix(), t2.StartedAt.Unix())
	assert.Equal(t, t1.NodeID, t2.NodeID)

	j2, err := ds.GetJobByID(ctx, j1.ID)
	assert.NoError(t, err)

	assert.Equal(t, tork.JobStateRunning, j2.State)
	assert.NoError(t, ds.Close())
}

func Test_handleStartedTaskOfFailedJob(t *testing.T) {
	ctx := context.Background()
	b := broker.NewInMemoryBroker()

	qname := uuid.NewUUID()

	cancellations := make(chan any)
	err := b.SubscribeForTasks(qname, func(t *tork.Task) error {
		close(cancellations)
		return nil
	})
	assert.NoError(t, err)

	ds, err := postgres.NewTestDatastore()
	assert.NoError(t, err)
	handler := NewStartedHandler(ds, b)
	assert.NotNil(t, handler)

	now := time.Now().UTC()

	j1 := &tork.Job{
		ID:    uuid.NewUUID(),
		State: tork.JobStateFailed,
	}
	err = ds.CreateJob(ctx, j1)
	assert.NoError(t, err)

	n1 := &tork.Node{
		ID:    uuid.NewUUID(),
		Queue: qname,
	}
	err = ds.CreateNode(ctx, n1)
	assert.NoError(t, err)

	t1 := &tork.Task{
		ID:        uuid.NewUUID(),
		State:     tork.TaskStateScheduled,
		StartedAt: &now,
		JobID:     j1.ID,
		NodeID:    n1.ID,
		CreatedAt: &now,
	}

	err = ds.CreateTask(ctx, t1)
	assert.NoError(t, err)

	err = handler(ctx, task.StateChange, t1)
	assert.NoError(t, err)

	<-cancellations

	t2, err := ds.GetTaskByID(ctx, t1.ID)
	assert.NoError(t, err)
	assert.Equal(t, tork.TaskStateScheduled, t2.State)
	assert.Equal(t, t1.StartedAt.Unix(), t2.StartedAt.Unix())
	assert.Equal(t, t1.NodeID, t2.NodeID)
	assert.NoError(t, ds.Close())
}
