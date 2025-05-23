package input

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/robfig/cron/v3"
	"github.com/runabol/tork"
	"github.com/runabol/tork/broker"
	"github.com/runabol/tork/datastore"
	"github.com/runabol/tork/internal/eval"
)

var (
	mountPattern = regexp.MustCompile(`^[-/\.0-9a-zA-Z_/= ]+$`)
)

func (ji Job) Validate(ds datastore.Datastore) error {
	validate := validator.New()
	if err := validate.RegisterValidation("duration", validateDuration); err != nil {
		return err
	}
	if err := validate.RegisterValidation("queue", validateQueue); err != nil {
		return err
	}
	if err := validate.RegisterValidation("expr", validateExpr); err != nil {
		return err
	}
	validate.RegisterStructValidation(validateMount, Mount{})
	validate.RegisterStructValidation(taskInputValidation, Task{})
	validate.RegisterStructValidation(validatePermission(ds), Permission{})
	return validate.Struct(ji)
}

func (ji ScheduledJob) Validate(ds datastore.Datastore) error {
	validate := validator.New()
	if err := validate.RegisterValidation("duration", validateDuration); err != nil {
		return err
	}
	if err := validate.RegisterValidation("queue", validateQueue); err != nil {
		return err
	}
	if err := validate.RegisterValidation("expr", validateExpr); err != nil {
		return err
	}
	validate.RegisterStructValidation(validateMount, Mount{})
	validate.RegisterStructValidation(taskInputValidation, Task{})
	validate.RegisterStructValidation(validatePermission(ds), Permission{})
	return validate.Struct(ji)
}

func validateExpr(fl validator.FieldLevel) bool {
	v := fl.Field().String()
	if v == "" {
		return true
	}
	return eval.ValidExpr(v)
}

func validateMount(sl validator.StructLevel) {
	mnt := sl.Current().Interface().(Mount)
	if mnt.Type == "" {
		sl.ReportError(mnt, "mount", "Mount", "typerequired", "")
	} else if mnt.Type == tork.MountTypeVolume && mnt.Source != "" {
		sl.ReportError(mnt, "mount", "Mount", "sourcenotempty", "")
	} else if mnt.Type == tork.MountTypeVolume && mnt.Target == "" {
		sl.ReportError(mnt, "mount", "Mount", "targetrequired", "")
	} else if mnt.Type == tork.MountTypeBind && mnt.Source == "" {
		sl.ReportError(mnt, "mount", "Mount", "sourcerequired", "")
	} else if mnt.Source != "" && !mountPattern.MatchString(mnt.Source) {
		sl.ReportError(mnt, "mount", "Mount", "invalidsource", "")
	} else if mnt.Target != "" && !mountPattern.MatchString(mnt.Target) {
		sl.ReportError(mnt, "mount", "Mount", "invalidtarget", "")
	} else if mnt.Target == "/tork" {
		sl.ReportError(mnt, "mount", "Mount", "invalidtarget", "")
	}
}

func validateCron(fl validator.FieldLevel) bool {
	v := fl.Field().String()
	_, err := cron.ParseStandard(v)
	return err == nil
}

func validatePermission(ds datastore.Datastore) func(sl validator.StructLevel) {
	return func(sl validator.StructLevel) {
		perm := sl.Current().Interface().(Permission)
		if perm.Role == "" && perm.User == "" {
			sl.ReportError(perm, "perm", "Permission", "roleoruser", "")
		}
		if perm.Role != "" && perm.User != "" {
			sl.ReportError(perm, "perm", "Permission", "roleoruser", "")
		}
		if perm.User != "" {
			if _, err := ds.GetUser(context.Background(), perm.User); err != nil {
				sl.ReportError(perm, "perm", "Permission", "invalidusername", "")
			}
		}
		if perm.Role != "" {
			if _, err := ds.GetRole(context.Background(), perm.Role); err != nil {
				sl.ReportError(perm, "perm", "Permission", "invalidrole", "")
			}
		}
	}
}

func validateDuration(fl validator.FieldLevel) bool {
	v := fl.Field().String()
	if v == "" {
		return true
	}
	_, err := time.ParseDuration(v)
	return err == nil
}

func validateQueue(fl validator.FieldLevel) bool {
	v := fl.Field().String()
	if v == "" {
		return true
	}
	if strings.HasPrefix(v, broker.QUEUE_EXCLUSIVE_PREFIX) {
		return false
	}
	if broker.IsCoordinatorQueue(v) {
		return false
	}
	return true
}

func taskInputValidation(sl validator.StructLevel) {
	taskTypeValidation(sl)
	compositeTaskValidation(sl)
}

func taskTypeValidation(sl validator.StructLevel) {
	ti := sl.Current().Interface().(Task)

	if ti.Parallel != nil && ti.Each != nil {
		sl.ReportError(ti.Each, "each", "Each", "paralleloreach", "")
		sl.ReportError(ti.Parallel, "parallel", "Parallel", "paralleloreach", "")
	}

	if ti.Parallel != nil && ti.SubJob != nil {
		sl.ReportError(ti.Each, "subjob", "SubJob", "parallelorsubjob", "")
		sl.ReportError(ti.Parallel, "parallel", "Parallel", "parallelorsubjob", "")
	}

	if ti.Each != nil && ti.SubJob != nil {
		sl.ReportError(ti.Each, "subjob", "SubJob", "eachorsubjob", "")
		sl.ReportError(ti.Parallel, "each", "Each", "eachorsubjob", "")
	}

}

func compositeTaskValidation(sl validator.StructLevel) {
	t := sl.Current().Interface().(Task)
	if t.Parallel == nil && t.Each == nil && t.SubJob == nil {
		return
	}
	if t.Image != "" {
		sl.ReportError(t.Parallel, "image", "Image", "invalidcompositetask", "")
	}
	if len(t.CMD) > 0 {
		sl.ReportError(t.Parallel, "cmd", "CMD", "invalidcompositetask", "")
	}
	if len(t.Entrypoint) > 0 {
		sl.ReportError(t.Entrypoint, "entrypoint", "Entrypoint", "invalidcompositetask", "")
	}
	if t.Run != "" {
		sl.ReportError(t.Run, "run", "Run", "invalidcompositetask", "")
	}
	if len(t.Env) > 0 {
		sl.ReportError(t.Env, "env", "Env", "invalidcompositetask", "")
	}
	if t.Queue != "" {
		sl.ReportError(t.Queue, "queue", "Queue", "invalidcompositetask", "")
	}
	if len(t.Pre) > 0 {
		sl.ReportError(t.Pre, "pre", "Pre", "invalidcompositetask", "")
	}
	if len(t.Post) > 0 {
		sl.ReportError(t.Post, "post", "Post", "invalidcompositetask", "")
	}
	if len(t.Mounts) > 0 {
		sl.ReportError(t.Mounts, "mounts", "Mounts", "invalidcompositetask", "")
	}
	if t.Retry != nil {
		sl.ReportError(t.Retry, "retry", "Retry", "invalidcompositetask", "")
	}
	if t.Limits != nil {
		sl.ReportError(t.Limits, "limits", "Limits", "invalidcompositetask", "")
	}
	if t.Timeout != "" {
		sl.ReportError(t.Timeout, "timeout", "Timeout", "invalidcompositetask", "")
	}
}
