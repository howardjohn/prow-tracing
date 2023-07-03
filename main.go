package main

import (
	"fmt"
	"github.com/howardjohn/prow-tracing/internal/gcs"
	"github.com/howardjohn/prow-tracing/internal/model"
	"github.com/howardjohn/prow-tracing/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/exp/slog"
	"os"
	"regexp"
	"time"
)

func main() {
	//job := os.Args[1]
	job := "istio-prow/pr-logs/pull/istio_istio/45746/integ-pilot_istio/1674927910177214464"
	// Hacky. If true, we just use the "job" span as the parent.
	// This allows multiple binaries to report to the same span.
	// TODO we probably want to report the root as one command, and each sub-command attaches. propogate parent ID by flag or env var.
	attach := len(os.Args) > 1 && os.Args[1] == "attach"
	client := gcs.NewClient(job)

	prowjob, err := gcs.Fetch[model.ProwJob](client, "prowjob.json")
	fatal(err)

	start, err := gcs.Fetch[model.Started](client, "started.json")
	fatal(err)

	finished, err := gcs.Fetch[model.Finished](client, "finished.json")
	fatal(err)

	pod, err := gcs.Fetch[model.PodReport](client, "podinfo.json")
	fatal(err)

	clone, err := gcs.Fetch[[]model.Record](client, "clone-records.json")
	fatal(err)

	slog.Info("running...")
	_ = prowjob
	_ = pod
	slog.Info("check", "start", fromEpoch(start.Timestamp), "pj", prowjob.CreationTimestamp.Time)
	slog.Info("check", "fin", fromEpoch(*finished.Timestamp), "pj", prowjob.CreationTimestamp.Time)

	root, shutdown, err := tracing.New(prowjob, attach)
	fatal(err)
	defer shutdown()

	podRecord := root.Recording("pod", pod.Pod.CreationTimestamp.Time, OrDefault(GetCondition(pod, "Ready"), fromEpoch(*finished.Timestamp)))
	for _, ev := range pod.Events {
		// Record all events as events. TODO: extract some of these like "pulled image" into spans.
		podRecord.Event(ev.Reason, ev.FirstTimestamp.Time, attribute.String("message", ev.Message))
	}
	podCtx := podRecord.End()

	if s := GetCondition(pod, "PodScheduled"); s != nil {
		podCtx.Record("pod/schedule", pod.Pod.CreationTimestamp.Time, *s)
	}

	for _, init := range pod.Pod.Status.InitContainerStatuses {
		if t := init.State.Terminated; t != nil {
			initCtx := podCtx.Record("init/"+init.Name, t.StartedAt.Time, t.FinishedAt.Time)
			switch init.Name {
			case "clonerefs":
				cur := t.StartedAt.Time
				for _, rec := range clone {
					if rec.Refs.Org == "" {
						continue
					}
					repoCtx := initCtx.Record(fmt.Sprintf("clone/%v/%v", rec.Refs.Org, rec.Refs.Repo), cur, cur.Add(rec.Duration))
					cmdTime := cur
					cur = cur.Add(rec.Duration)
					for _, cmd := range rec.Commands {
						repoCtx.Record(classifyGitCommand(cmd.Command), cmdTime, cmdTime.Add(cmd.Duration))
						cmdTime = cmdTime.Add(cmd.Duration)
					}
				}
			}
		}
	}
	for _, c := range pod.Pod.Status.ContainerStatuses {
		if t := c.State.Terminated; t != nil {
			podCtx.Record("container/"+c.Name, t.StartedAt.Time, t.FinishedAt.Time)
		}
	}
}

func GetCondition(pod model.PodReport, cond string) *time.Time {
	for _, c := range pod.Pod.Status.Conditions {
		if c.Type == cond {
			return &c.LastTransitionTime.Time
		}
	}
	return nil
}

var commandRegexp = regexp.MustCompile("git (.+?)( |$)")

func classifyGitCommand(command string) string {
	m := commandRegexp.FindStringSubmatch(command)
	if len(m) > 0 {
		return m[0]
	}
	slog.Info("unknown git command", "cmd", command)
	return "unknown"
}

func fromEpoch(i int64) time.Time {
	return time.Unix(i, 0)
}

func fatal(err error) {
	if err == nil {
		return
	}
	panic(err.Error())
}

// OrDefault returns *t if its non-nil, or else def.
func OrDefault[T any](t *T, def T) T {
	if t != nil {
		return *t
	}
	return def
}
