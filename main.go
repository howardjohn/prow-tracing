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
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}
	switch cmd {
	case "", "prowjob":
		prowjob(args)
	case "test":
		test(args)
	}
}

func test(args []string) {
	uid := os.Getenv("PROW_JOB_ID")
	if uid == "" {
		fatal(fmt.Errorf("PROW_JOB_ID required"))
	}
	root, shutdown, err := tracing.NewAction(uid)
	fatal(err)
	defer shutdown()
	t0 := time.Date(2023, time.July, 01, 0, 0, 0, 0, time.UTC)
	ec := root.Record("setup cluster", t0, t0.Add(time.Minute))
	ec.Record("pull image", t0.Add(time.Second), t0.Add(time.Second*10))
	cf := root.Record("test/conformance", t0.Add(time.Minute), t0.Add(time.Minute*12))
	cf1 := cf.Record("test/conformance/subtest1", t0.Add(time.Minute+time.Second), t0.Add(time.Minute+time.Second*7))
	cf1.Record("test/conformance/subtest1/apply-config", t0.Add(time.Minute+time.Second*2), t0.Add(time.Minute+time.Second*3))

	cf.Record("test/conformance/subtest2", t0.Add(time.Minute+time.Second*8), t0.Add(time.Minute+time.Second*15))
}

func prowjob(args []string) {
	// Temporary hard coded test job
	job := "istio-prow/pr-logs/pull/istio_istio/45746/integ-pilot_istio/1674927910177214464"
	if len(args) > 0 {
		job = args[1]
	}
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

	trace, shutdown, err := tracing.NewRoot(prowjob)
	fatal(err)
	defer shutdown()

	root := trace.Record("job", prowjob.Status.StartTime.Time, prowjob.Status.CompletionTime.Time)

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
