package model

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

// Started holds the started.json values of the build.
type Started struct {
	// Timestamp is UTC epoch seconds when the job started.
	Timestamp int64 `json:"timestamp"` // epoch seconds
	// Node holds the name of the machine that ran the job.
	Node string `json:"node,omitempty"`

	// Consider whether to keep the following:

	// Pull holds the PR number the primary repo is testing
	Pull string `json:"pull,omitempty"`
	// Repos holds the RepoVersion of all commits checked out.
	Repos      map[string]string `json:"repos,omitempty"` // {repo: branch_or_pull} map
	RepoCommit string            `json:"repo-commit,omitempty"`
}

// Finished holds the finished.json values of the build
type Finished struct {
	// Timestamp is UTC epoch seconds when the job finished.
	// An empty value indicates an incomplete job.
	Timestamp *int64 `json:"timestamp,omitempty"`
	// Passed is true when the job completes successfully.
	Passed *bool `json:"passed"`
}

type Metadata map[string]interface{}

type PodReport struct {
	Pod    *Pod    `json:"pod,omitempty"`
	Events []Event `json:"events,omitempty"`
}

type Pod struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            PodStatus `json:"status,omitempty"`
}
type PodStatus struct {

	// The list has one entry per init container in the manifest. The most recent successful
	// init container will have ready = true, the most recently started container will have
	// startTime set.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty"`

	// The list has one entry per container in the manifest.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status
	// +optional
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
	Conditions        []PodCondition    `json:"conditions,omitempty" `
}
type PodCondition struct {
	Type               string      `json:"type"`
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}
type Event struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// This should be a short, machine understandable string that gives the reason
	// for the transition into the object's current status.
	// TODO: provide exact specification for format.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human-readable description of the status of this operation.
	// TODO: decide on maximum length.
	// +optional
	Message string `json:"message,omitempty"`

	// The time at which the event was first recorded. (Time of server receipt is in TypeMeta.)
	// +optional
	FirstTimestamp metav1.Time `json:"firstTimestamp,omitempty"`

	// The time at which the most recent occurrence of this event was recorded.
	// +optional
	LastTimestamp metav1.Time `json:"lastTimestamp,omitempty"`

	// The number of times this event has occurred.
	// +optional
	Count int32 `json:"count,omitempty"`

	// Type of this event (Normal, Warning), new types could be added in the future
	// +optional
	Type string `json:"type,omitempty"`

	// Time when this Event was first observed.
	// +optional
	EventTime metav1.MicroTime `json:"eventTime,omitempty"`

	// What action was taken/failed regarding to the Regarding object.
	// +optional
	Action string `json:"action,omitempty"`

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	// +optional
	ReportingController string `json:"reportingComponent"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	// +optional
	ReportingInstance string `json:"reportingInstance"`
}

type ProwJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            ProwJobStatus `json:"status,omitempty"`
}
type ProwJobStatus struct {
	// StartTime is equal to the creation time of the ProwJob
	StartTime metav1.Time `json:"startTime,omitempty"`
	// PendingTime is the timestamp for when the job moved from triggered to pending
	PendingTime *metav1.Time `json:"pendingTime,omitempty"`
	// CompletionTime is the timestamp for when the job goes to a final state
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// ContainerStatus contains details for the current status of this container.
type ContainerStatus struct {
	// This must be a DNS_LABEL. Each container in a pod must have a unique name.
	// Cannot be updated.
	Name string `json:"name"`
	// Details about the container's current condition.
	// +optional
	State ContainerState `json:"state,omitempty"`
	// Specifies whether the container has passed its readiness probe.
	Ready bool `json:"ready"`
	// The number of times the container has been restarted.
	RestartCount int32 `json:"restartCount"`
	// The image the container is running.
	// More info: https://kubernetes.io/docs/concepts/containers/images.
	Image string `json:"image"`
	// ImageID of the container's image.
	ImageID string `json:"imageID"`
	// Container's ID in the format '<type>://<container_id>'.
	// +optional
	ContainerID string `json:"containerID,omitempty"`
	// Specifies whether the container has passed its startup probe.
	// Initialized as false, becomes true after startupProbe is considered successful.
	// Resets to false when the container is restarted, or if kubelet loses state temporarily.
	// Is always true when no startupProbe is defined.
	// +optional
	Started *bool `json:"started,omitempty"`
}

// ContainerState holds a possible state of container.
// Only one of its members may be specified.
// If none of them is specified, the default one is ContainerStateWaiting.
type ContainerState struct {
	// Details about a terminated container
	// +optional
	Terminated *ContainerStateTerminated `json:"terminated,omitempty"`
	// Details about a running container
	// +optional
	Running *ContainerStateRunning `json:"running,omitempty"`
}

type ContainerStateRunning struct {
	// Time at which the container was last (re-)started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty"`
}

// ContainerStateTerminated is a terminated state of a container.
type ContainerStateTerminated struct {
	// Exit status from the last termination of the container
	ExitCode int32 `json:"exitCode"`
	// Signal from the last termination of the container
	// +optional
	Signal int32 `json:"signal,omitempty"`
	// (brief) reason from the last termination of the container
	// +optional
	Reason string `json:"reason,omitempty"`
	// Message regarding the last termination of the container
	// +optional
	Message string `json:"message,omitempty"`
	// Time at which previous execution of the container started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty"`
	// Time at which the container last terminated
	// +optional
	FinishedAt metav1.Time `json:"finishedAt,omitempty"`
	// Container's ID in the format '<type>://<container_id>'
	// +optional
	ContainerID string `json:"containerID,omitempty"`
}

// Record is a trace of what the desired
// git state was, what steps we took to get there,
// what our final state ended up being, and
// whether or not we were successful.
type Record struct {
	Refs     Refs      `json:"refs"`
	Commands []Command `json:"commands,omitempty"`
	Failed   bool      `json:"failed,omitempty"`

	// FinalSHA is the SHA from ultimate state of a cloned ref
	// This is used to populate RepoCommit in started.json properly
	FinalSHA string `json:"final_sha,omitempty"`

	// Duration is the total runtime for the clone.
	Duration time.Duration `json:"duration,omitempty"`
}

// Command is a trace of a command executed
// while achieving the desired git state.
type Command struct {
	Command string `json:"command"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	// Duration is the runtime for the command.
	Duration time.Duration `json:"duration,omitempty"`
}
type Refs struct {
	// Org is something like kubernetes or k8s.io
	Org string `json:"org"`
	// Repo is something like test-infra
	Repo string `json:"repo"`
	// RepoLink links to the source for Repo.
	RepoLink string `json:"repo_link,omitempty"`

	BaseRef string `json:"base_ref,omitempty"`
	BaseSHA string `json:"base_sha,omitempty"`
	// BaseLink is a link to the commit identified by BaseSHA.
	BaseLink string `json:"base_link,omitempty"`

	// PathAlias is the location under <root-dir>/src
	// where this repository is cloned. If this is not
	// set, <root-dir>/src/github.com/org/repo will be
	// used as the default.
	PathAlias string `json:"path_alias,omitempty"`

	// WorkDir defines if the location of the cloned
	// repository will be used as the default working
	// directory.
	WorkDir bool `json:"workdir,omitempty"`

	// CloneURI is the URI that is used to clone the
	// repository. If unset, will default to
	// `https://github.com/org/repo.git`.
	CloneURI string `json:"clone_uri,omitempty"`
	// SkipSubmodules determines if submodules should be
	// cloned when the job is run. Defaults to false.
	SkipSubmodules bool `json:"skip_submodules,omitempty"`
	// CloneDepth is the depth of the clone that will be used.
	// A depth of zero will do a full clone.
	CloneDepth int `json:"clone_depth,omitempty"`
	// SkipFetchHead tells prow to avoid a git fetch <remote> call.
	// Multiheaded repos may need to not make this call.
	// The git fetch <remote> <BaseRef> call occurs regardless.
	SkipFetchHead bool `json:"skip_fetch_head,omitempty"`
}
