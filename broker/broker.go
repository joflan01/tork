package broker

import (
	"context"

	"github.com/runabol/tork"
)

type Provider func() (Broker, error)

const (
	BROKER_INMEMORY     = "inmemory"
	BROKER_RABBITMQ     = "rabbitmq"
	TOPIC_JOB           = "job.*"
	TOPIC_JOB_COMPLETED = "job.completed"
	TOPIC_JOB_FAILED    = "job.failed"
	TOPIC_SCHEDULED_JOB = "scheduled.job"
)

// Broker is the message-queue, pub/sub mechanism used for delivering tasks.
type Broker interface {
	PublishTask(ctx context.Context, qname string, t *tork.Task) error
	SubscribeForTasks(qname string, handler func(t *tork.Task) error) error

	PublishTaskProgress(ctx context.Context, t *tork.Task) error
	SubscribeForTaskProgress(handler func(t *tork.Task) error) error

	PublishHeartbeat(ctx context.Context, n *tork.Node) error
	SubscribeForHeartbeats(handler func(n *tork.Node) error) error

	PublishJob(ctx context.Context, j *tork.Job) error
	SubscribeForJobs(handler func(j *tork.Job) error) error

	PublishEvent(ctx context.Context, topic string, event any) error
	SubscribeForEvents(ctx context.Context, pattern string, handler func(event any)) error

	PublishTaskLogPart(ctx context.Context, p *tork.TaskLogPart) error
	SubscribeForTaskLogPart(handler func(p *tork.TaskLogPart)) error

	Queues(ctx context.Context) ([]QueueInfo, error)
	HealthCheck(ctx context.Context) error
	Shutdown(ctx context.Context) error
}
