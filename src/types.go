package app

const (
	defaultPollIntervalSeconds = 30
	defaultMaxPolls            = 120
	defaultOutputStaleSeconds  = 240
	defaultHeartbeatStaleSecs  = 8
	defaultStateLockTimeoutMS  = 2500
	defaultEventLockTimeoutMS  = 2500
	defaultEventsMaxBytes      = 1_000_000
	defaultEventsMaxLines      = 2000
	defaultEventRetentionDays  = 14
	defaultProcessScanInterval = 8
	defaultProcessListCacheMS  = 500
	defaultCmdTimeoutSeconds   = 20
	defaultTmuxWidth           = 220
	defaultTmuxHeight          = 60
	maxInlineSendLength        = 500
	execDonePrefix             = "__LISA_EXEC_DONE__:"
	sessionStartPrefix         = "__LISA_SESSION_START__:"
	sessionDonePrefix          = "__LISA_SESSION_DONE__:"
)

type sessionMeta struct {
	Session             string `json:"session"`
	ParentSession       string `json:"parentSession,omitempty"`
	Agent               string `json:"agent"`
	Mode                string `json:"mode"`
	Lane                string `json:"lane,omitempty"`
	OAuthTokenID        string `json:"oauthTokenId,omitempty"`
	RunID               string `json:"runId,omitempty"`
	ProjectRoot         string `json:"projectRoot"`
	SocketPath          string `json:"socketPath,omitempty"`
	StartCmd            string `json:"startCommand"`
	Prompt              string `json:"prompt,omitempty"`
	ObjectiveID         string `json:"objectiveId,omitempty"`
	ObjectiveGoal       string `json:"objectiveGoal,omitempty"`
	ObjectiveAcceptance string `json:"objectiveAcceptance,omitempty"`
	ObjectiveBudget     int    `json:"objectiveBudget,omitempty"`
	CreatedAt           string `json:"createdAt"`
}

type sessionState struct {
	PollCount                  int     `json:"pollCount"`
	HasEverBeenActive          bool    `json:"hasEverBeenActive"`
	LastResolvedAgent          string  `json:"lastResolvedAgent,omitempty"`
	LastResolvedMode           string  `json:"lastResolvedMode,omitempty"`
	LastOutputHash             string  `json:"lastOutputHash"`
	LastOutputAt               int64   `json:"lastOutputAt"`
	LastOutputAtNanos          int64   `json:"lastOutputAtNanos,omitempty"`
	LastInputAt                int64   `json:"lastInputAt,omitempty"`
	LastInputAtNanos           int64   `json:"lastInputAtNanos,omitempty"`
	LastAgentPID               int     `json:"lastAgentPid,omitempty"`
	LastAgentProbeAt           int64   `json:"lastAgentProbeAt,omitempty"`
	LastAgentCPU               float64 `json:"lastAgentCpu,omitempty"`
	LastTurnCompleteAtNanos    int64   `json:"lastTurnCompleteAtNanos,omitempty"`
	LastTurnCompleteInputNanos int64   `json:"lastTurnCompleteInputNanos,omitempty"`
	LastTurnCompleteFileAge    int     `json:"lastTurnCompleteFileAge,omitempty"`
	OAuthTokenPruned           bool    `json:"oauthTokenPruned,omitempty"`
	OAuthTokenPruneReason      string  `json:"oauthTokenPruneReason,omitempty"`
	LastSessionState           string  `json:"lastSessionState,omitempty"`
	LastStatus                 string  `json:"lastStatus,omitempty"`
	LastClassificationReason   string  `json:"lastClassificationReason,omitempty"`
	LastClassificationPollRef  int     `json:"lastClassificationPollRef,omitempty"`
	ClaudeSessionID            string  `json:"claudeSessionId,omitempty"`
	CodexSessionID             string  `json:"codexSessionId,omitempty"`
}

type sessionStatus struct {
	Session              string        `json:"session"`
	Agent                string        `json:"agent"`
	Mode                 string        `json:"mode"`
	Status               string        `json:"status"`
	TodosDone            int           `json:"todosDone"`
	TodosTotal           int           `json:"todosTotal"`
	ActiveTask           string        `json:"activeTask"`
	WaitEstimate         int           `json:"waitEstimate"`
	SessionState         string        `json:"sessionState"`
	PaneStatus           string        `json:"paneStatus"`
	PaneCommand          string        `json:"paneCommand"`
	AgentPID             int           `json:"agentPid"`
	AgentCPU             float64       `json:"agentCpu"`
	OutputAgeSeconds     int           `json:"outputAgeSeconds"`
	OutputFreshSeconds   int           `json:"outputFreshSeconds"`
	HeartbeatAge         int           `json:"heartbeatAgeSeconds"`
	HeartbeatFreshSecs   int           `json:"heartbeatFreshSeconds"`
	ClassificationReason string        `json:"classificationReason"`
	Signals              statusSignals `json:"signals"`
	OutputFile           string        `json:"outputFile,omitempty"`
}

type statusSignals struct {
	RunID                    string `json:"runId,omitempty"`
	DoneFileSeen             bool   `json:"doneFileSeen"`
	DoneFileRunID            string `json:"doneFileRunId,omitempty"`
	DoneFileRunMismatch      bool   `json:"doneFileRunMismatch"`
	DoneFileExitCode         int    `json:"doneFileExitCode"`
	DoneFileReadError        string `json:"doneFileReadError,omitempty"`
	SessionMarkerSeen        bool   `json:"sessionMarkerSeen"`
	SessionMarkerRunID       string `json:"sessionMarkerRunId,omitempty"`
	SessionMarkerRunMismatch bool   `json:"sessionMarkerRunMismatch"`
	SessionExitCode          int    `json:"sessionExitCode"`
	ExecMarkerSeen           bool   `json:"execMarkerSeen"`
	ExecExitCode             int    `json:"execExitCode"`
	PromptWaiting            bool   `json:"promptWaiting"`
	InteractiveWaiting       bool   `json:"interactiveWaiting"`
	ActiveProcessBusy        bool   `json:"activeProcessBusy"`
	AgentProcessDetected     bool   `json:"agentProcessDetected"`
	OutputFresh              bool   `json:"outputFresh"`
	HeartbeatSeen            bool   `json:"heartbeatSeen"`
	HeartbeatFresh           bool   `json:"heartbeatFresh"`
	PaneIsShell              bool   `json:"paneIsShell"`
	AgentScanCached          bool   `json:"agentScanCached"`
	AgentScanError           string `json:"agentScanError,omitempty"`
	TranscriptTurnComplete   bool   `json:"transcriptTurnComplete"`
	TranscriptFileAge        int    `json:"transcriptFileAge"`
	TranscriptError          string `json:"transcriptError,omitempty"`
	StateLockWaitMS          int    `json:"stateLockWaitMs"`
	StateLockTimedOut        bool   `json:"stateLockTimedOut"`
	TMUXReadError            string `json:"tmuxReadError,omitempty"`
	MetaReadError            string `json:"metaReadError,omitempty"`
	StateReadError           string `json:"stateReadError,omitempty"`
	EventsWriteError         string `json:"eventsWriteError,omitempty"`
}

type sessionEvent struct {
	At      string        `json:"at"`
	Type    string        `json:"type"`
	Session string        `json:"session"`
	State   string        `json:"state"`
	Status  string        `json:"status"`
	Reason  string        `json:"reason"`
	Poll    int           `json:"poll"`
	Signals statusSignals `json:"signals"`
}

type monitorResult struct {
	FinalState  string `json:"finalState"`
	Session     string `json:"session"`
	TodosDone   int    `json:"todosDone"`
	TodosTotal  int    `json:"todosTotal"`
	OutputFile  string `json:"outputFile,omitempty"`
	NextOffset  int    `json:"nextOffset,omitempty"`
	ExitReason  string `json:"exitReason"`
	Polls       int    `json:"polls"`
	FinalStatus string `json:"finalStatus"`
}

type processInfo struct {
	PID     int
	PPID    int
	CPU     float64
	Command string
}
